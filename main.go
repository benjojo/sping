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

	// fmt.Printf("[%v] %#v\n", rxAddr.String(), rx)
	log.Printf("RX diff is %s", timeRX.Sub(rx.TXTime))
	pI := pingInfo{
		ID: rx.ID,
		TX: rx.TXTime,
		RX: timeRX,
	}

	session := sessionMap[rx.Session]
	session.LastAcks[session.getNextAckSlot()] = pI
	sessionMap[rx.Session] = session

	for n, v := range session.LastAcks {
		fmt.Printf("\t[Slot %d] ID: %d - TX: %s\n", n, v.ID, v.TX.Sub(v.RX))
	}
}

/*
	[Slot 0]  ID: 225 - TX: -52.440514ms
	[Slot 1]  ID: 226 - TX: -52.684686ms
	[Slot 2]  ID: 227 - TX: -52.85241ms
	[Slot 3]  ID: 228 - TX: -56.673241ms
	[Slot 4]  ID: 229 - TX: -52.393849ms
	[Slot 5]  ID: 230 - TX: -52.520913ms
	[Slot 6]  ID: 231 - TX: -54.022304ms
	[Slot 7]  ID: 232 - TX: -52.65099ms
	[Slot 8]  ID: 233 - TX: -52.424397ms
	[Slot 9]  ID: 234 - TX: -52.13888ms
	[Slot 10] ID: 235 - TX: -52.338799ms
	[Slot 11] ID: 236 - TX: -52.595029ms
	[Slot 12] ID: 237 - TX: -54.515562ms
	[Slot 13] ID: 238 - TX: -52.545304ms
	[Slot 14] ID: 239 - TX: -53.134174ms
	[Slot 15] ID: 240 - TX: -52.737777ms
	[Slot 16] ID: 241 - TX: -53.157872ms
	[Slot 17] ID: 242 - TX: -52.686319ms
	[Slot 18] ID: 243 - TX: -52.347642ms
	[Slot 19] ID: 244 - TX: -52.32119ms
	[Slot 20] ID: 245 - TX: -52.745323ms
	[Slot 21] ID: 246 - TX: -52.505479ms
	[Slot 22] ID: 247 - TX: -52.736172ms
	[Slot 23] ID: 248 - TX: -52.727409ms
	[Slot 24] ID: 249 - TX: -52.374326ms
	[Slot 25] ID: 250 - TX: -52.446681ms
	[Slot 26] ID: 251 - TX: -52.671936ms
	[Slot 27] ID: 252 - TX: -52.591185ms
	[Slot 28] ID: 253 - TX: -52.893442ms
	[Slot 29] ID: 254 - TX: -52.105233ms
	[Slot 30] ID: 255 - TX: -52.46667ms
	[Slot 31] ID: 148 - TX: -52.599296ms
*/

func nextAckSlot(in [32]pingInfo) int {
	lowestN := 0
	lowestID := uint8(255)

	for n, v := range in {
		if v.ID == 0 {
			return n
		}

		if v.ID < lowestID {
			lowestN = n
			lowestID = v.ID
		}
	}

	log.Printf("next slot:%d", lowestN)

	return lowestN
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
