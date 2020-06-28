package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/vmihailenco/msgpack/v4"
	"golang.org/x/time/rate"
)

/*
 [+] Auth packet
	[+] Respond with cookie to use
	[+] If failed, just don't repsond
 [+] Ping packets
*/
var packetLimiter = rate.NewLimiter(100, 300)

var debugFlagSlotShow = flag.Bool("debug.showslots", false, "Show incoming packet latency slots")

func main() {
	udpPPSin := flag.Int("udp.pps", 100, "max inbound PPS that can be processed at once")
	peers := flag.String("peers", "", "List of IPs that are peers")

	flag.Parse()

	sessionMap = make(map[uint32]*session)

	packetLimiter = rate.NewLimiter(rate.Limit(*udpPPSin), (*udpPPSin)*3)
	getTimeOffset()

	go listenOnTCP()
	go listenAndRoute()

	if len(*peers) != 0 {
		peerList := strings.Split(*peers, ",")
		for _, v := range peerList {
			ip := net.ParseIP(v)
			if ip != nil {
				// Start a session with this host
				go startSession(v)
			}
		}
	}

	for {
		a := timeNowCorrected().Unix()
		u := time.Until(time.Unix(a+1, 0).Add(timeOffset * -1))
		time.Sleep(u)
		fmt.Printf("it is now: %s\n", time.Now())
	}
}

type session struct {
	Init        bool
	PeerAddress net.IP
	LastAcks    [32]pingInfo
	LastRX      time.Time
	CurrentID   uint8
	SessionID   uint32
	nextAckSlot int
}

func (s *session) getNextAckSlot() int {
	if s.nextAckSlot != 31 {
		s.nextAckSlot = s.nextAckSlot + 1
	} else {
		s.nextAckSlot = 0
	}
	return s.nextAckSlot
}

func (s *session) sendPackets() {
	peerAddr, err := net.ResolveUDPAddr("udp", s.PeerAddress.String()+":6924")
	if err != nil {
		log.Fatalf("Failed to parse peer address %v / %#v", err, s)
	}
	udpConn, err := net.DialUDP("udp", nil, peerAddr)
	if err != nil {
		log.Printf("Failed to setup packet sending to %#v", s)
		return
	}

	startTime := time.Now()
	for {
		a := timeNowCorrected().Unix()
		u := time.Until(time.Unix(a+1, 0).Add(timeOffset * -1))
		time.Sleep(u)

		if !s.Init {
			if time.Since(startTime) > time.Second*30 {
				// TODO: Handle this better
				return
			}
		}

		// Send pings
		s.CurrentID++
		packet := pingStruct{
			Magic:    11181,
			Session:  s.SessionID,
			ID:       s.CurrentID,
			TXTime:   timeNowCorrected(),
			LastAcks: s.LastAcks,
		}

		b, err := msgpack.Marshal(packet)
		if err != nil {
			log.Fatalf("Failed to marshal packet %v / %#v", err, packet)
		}

		udpConn.Write(b)
	}
}

func startSession(host string) {
	ip := net.ParseIP(host)
	if ip == nil {
		log.Printf("%#v is not a valid IP address", host)
		return
	}

	conn, err := net.Dial("tcp", host+":6924")
	if err != nil {
		log.Printf("Cannot connect to %v: %v", host, err)
		return
	}

	bannerBuf := make([]byte, 10000)

	n, err := conn.Read(bannerBuf)
	if n > 9000 {
		log.Printf("%v: Host banner too big", host)
		conn.Close()
		return
	}

	if !strings.HasPrefix(string(bannerBuf[:n]), "sping-") {
		log.Printf("%v: Host banner not sping", host)
		conn.Close()
		return
	}

	defer conn.Close()

	startPacket := handshakeStruct{
		Magic:   11181,
		Version: 1,
		Session: 0,
	}

	b, err := msgpack.Marshal(startPacket)
	if err == nil {
		conn.Write(b)
	} else {
		return
	}

	initBuf := make([]byte, 10000)

	n, err = conn.Read(initBuf)
	if err != nil {
		log.Printf("Handshake failed to %v: %v", host, err)
		return
	}
	initPacket := handshakeStruct{}
	err = msgpack.Unmarshal(initBuf[:n], &initPacket)
	if err != nil {
		log.Printf("Corrupt handshake from %v: %v", host, err)
		return
	}

	sessionLock.Lock()
	sessionMap[initPacket.Session] = &session{
		Init:        false,
		PeerAddress: conn.RemoteAddr().(*net.TCPAddr).IP,
		SessionID:   initPacket.Session,
	}
	sessionLock.Unlock()
	go sessionMap[initPacket.Session].sendPackets()
}

var sessionMap map[uint32]*session
var sessionLock sync.RWMutex

func listenAndRoute() {
	uListener, err := net.ListenPacket("udp", "[::]:6924")
	if err != nil {
		log.Fatalf("Failed to listen on UDP port %v", err)
	}

	for {
		buf := make([]byte, 10000)
		n, rxAddr, err := uListener.ReadFrom(buf)

		if err != nil {
			log.Fatalf("Failed to rx from UDP, %v", err)
		}

		if !packetLimiter.Allow() {
			continue
		}

		go handlePacket(buf[:n], rxAddr)
	}
}

func handlePacket(buf []byte, rxAddr net.Addr) {
	timeRX := timeNowCorrected()

	rx := pingStruct{}
	err := msgpack.Unmarshal(buf, &rx)
	if err != nil {
		log.Printf("Failed to parse packet from %v", rxAddr.String())
		return
	}

	if rx.Magic != 11181 {
		log.Printf("Invalid magic from %v", rxAddr.String())
		return
	}

	if sessionMap[rx.Session] != nil {
		if !sessionMap[rx.Session].Init {
			log.Printf("Setting session as active")
			if sessionMap[rx.Session].SessionID != rx.Session {
				log.Printf("Invalid packet for the session ID from %v", rxAddr.String())
				return
			}
			ses := sessionMap[rx.Session]
			ses.Init = true
			sessionMap[rx.Session] = ses
		}
	} else {
		return
	}

	pI := pingInfo{
		ID: rx.ID,
		TX: rx.TXTime,
		RX: timeRX,
	}

	session := sessionMap[rx.Session]
	RXL, TXL, _, _, _ := getStats(timeRX, rx, session)
	log.Printf("[%s] RX: %s TX: %s ", session.PeerAddress, RXL, TXL) // [Loss RX: %d/%d | Loss TX %d/%d]

	session.LastAcks[session.getNextAckSlot()] = pI
	sessionMap[rx.Session] = session

	if *debugFlagSlotShow {
		for n, v := range session.LastAcks {
			fmt.Printf("\t[Slot %d] ID: %d - TX: %s\n", n, v.ID, v.TX.Sub(v.RX))
		}
	}
}

func getStats(timeRX time.Time, rx pingStruct, ses *session) (RXLatency time.Duration, TXLatency time.Duration, RXLoss int, TXLoss int, TotalSent int) {
	RXLatency = timeRX.Sub(rx.TXTime)

	latest := time.Hour * 24
	for _, v := range rx.LastAcks {
		if v.RX.IsZero() {
			continue
		}

		if time.Since(v.TX) < latest {
			TXLatency = v.RX.Sub(v.TX)
			latest = time.Since(v.TX)
		}
	}

	return RXLatency, TXLatency, 0, 0, 0
}

type pingStruct struct {
	Magic        uint16       `msgpack:"M"`
	Session      uint32       `msgpack:"S"`
	ID           uint8        `msgpack:"I"`
	TXTime       time.Time    `msgpack:"T"`
	SendersError uint16       `msgpack:"E"`
	LastAcks     [32]pingInfo `msgpack:"A"`
}

type pingInfo struct {
	ID uint8     `msgpack:"R"`
	TX time.Time `msgpack:"U"` // As given by the senders PingStruct
	RX time.Time `msgpack:"X"` // As read by the rx's ingress
}
