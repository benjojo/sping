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

var packetLimiter = rate.NewLimiter(100, 300)

var debugFlagSlotShow = flag.Bool("debug.showslots", false, "Show incoming packet latency slots")
var debugShowLiveStats = flag.Bool("debug.showstats", false, "Show per ping info, and timestamps")

func main() {
	udpPPSin := flag.Int("udp.pps", 100, "max inbound PPS that can be processed at once")
	peers := flag.String("peers", "", "List of IPs that are peers")
	flag.Parse()

	if *usePPS && !*flagClockIsPerfect {
		*flagClockIsPerfect = true
		log.Printf("PPS mode is in use, Automatically assuming system clock is perfect")
	}

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
				go startSession(ip)
			}
		}
	}

	go sessionGC()

	if *usePPS {
		go ppsClockTicker()
	} else {
		go sysClockTicker()
	}

	go handlePrometheus()
	for {

		a := timeNowCorrected().Unix()
		u := time.Until(time.Unix(a+1, 0).Add(timeOffset * -1))
		time.Sleep(u)
		if *debugShowLiveStats {
			fmt.Printf("it is now: %s\n", time.Now())
		}
	}
}

func ppsClockTicker() {
	sessionList := make([]*session, 0)
	setupPPS()

	for {
		waitForPPSPulse()

		for _, v := range sessionList {
			select {
			case v.pulse <- true:
			default:
				// Well /shrug I guess
			}
		}

		sessionLock.Lock()
		sessionList = make([]*session, 0)
		for _, v := range sessionMap {
			sessionList = append(sessionList, v)
		}
		sessionLock.Unlock()
	}
}

func sysClockTicker() {
	sessionList := make([]*session, 0)

	for {
		a := timeNowCorrected().Unix()
		u := time.Until(time.Unix(a+1, 0).Add(timeOffset * -1))
		time.Sleep(u)

		for _, v := range sessionList {
			select {
			case v.pulse <- true:
			default:
				// Well /shrug I guess
			}
		}

		sessionLock.Lock()
		sessionList = make([]*session, 0)
		for _, v := range sessionMap {
			sessionList = append(sessionList, v)
		}
		sessionLock.Unlock()
	}
}

func sessionGC() {
	for {
		time.Sleep(time.Minute)
		sessionLock.Lock()
		for ID, ses := range sessionMap {
			if time.Since(ses.LastRX) > time.Minute {
				log.Printf("GC - Session with %s for inactivity", ses.PeerAddress)
				delete(sessionMap, ID)
				continue
			}
			if !ses.UDPActivated {
				if time.Since(ses.SessionMade) > time.Second*20 {
					log.Printf("GC - Session with %s for lack of handshake", ses.PeerAddress)
					delete(sessionMap, ID)
					continue
				}
			}
		}
		sessionLock.Unlock()
	}
}

type session struct {
	TCPActivated bool      // aka it's been made after a TCP Handshake
	UDPActivated bool      // aka it's been confirmed with a UDP Handshake
	MadeByMe     bool      // If I made the session, aka if I should send the UDP Handshake
	UDPHandshake chan bool // Used to confirm a UDP handshake
	PeerAddress  net.IP    // Used only to start a session
	SessionMade  time.Time // Used to eventually give up on a session

	// Network Mobility data
	ReplyWith net.PacketConn
	ReplyTo   *net.UDPAddr

	// Time keeping data
	LastAcks    [32]pingInfo
	LastRX      time.Time
	CurrentID   uint8
	SessionID   uint32
	nextAckSlot int
	LastRXPing  pingStruct

	// Time pulse channel
	pulse chan bool
}

var globalReplyWith *net.PacketConn

func (s *session) getNextAckSlot() int {
	if s.nextAckSlot != 31 {
		s.nextAckSlot = s.nextAckSlot + 1
	} else {
		s.nextAckSlot = 0
	}
	return s.nextAckSlot
}

func (s *session) sendPackets() {
	timeStarted := time.Now()
	for {
		if *usePPS {
			waitForPPSPulse()
		} else {
			a := timeNowCorrected().Unix()
			u := time.Until(time.Unix(a+1, 0).Add(timeOffset * -1))
			time.Sleep(u)
		}

		if (time.Since(s.LastRX) > time.Second*60) && (time.Since(timeStarted) > time.Second*10) {
			return
		}

		// Send pings
		s.CurrentID = uint8(time.Now().Unix()%255) + 1
		packet := pingStruct{
			Type:     't',
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

		if s.ReplyTo != nil {
			s.ReplyWith.WriteTo(b, s.ReplyTo)
		} else {
			log.Printf("s.ReplyTo is nil")
		}
	}
}

var sessionMap map[uint32]*session
var sessionLock sync.RWMutex

var bindAddr = flag.String("listenAddr", "[::]:6924", "Listening address")

func listenAndRoute() {
	uListener, err := net.ListenPacket("udp", *bindAddr)
	if err != nil {
		log.Fatalf("Failed to listen on UDP port %v", err)
	}

	globalReplyWith = &uListener

	for {
		buf := make([]byte, 10000)
		n, rxAddr, err := uListener.ReadFrom(buf)

		if err != nil {
			log.Fatalf("Failed to rx from UDP, %v", err)
			time.Sleep(time.Millisecond * 777)
		}

		if !packetLimiter.Allow() {
			continue
		}

		go handlePacket(buf[:n], rxAddr.(*net.UDPAddr), uListener)
	}
}

func handlePacket(buf []byte, rxAddr *net.UDPAddr, lSocket net.PacketConn) {
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

	if rx.Type == 'h' {
		// Differnet handler for handshakes
		handleInboundHandshake(buf, rxAddr, lSocket)
		return
	}
	if rx.Type != 't' {
		// It's not a time packet? Must be corrupted then
		log.Printf("Corrupted Packet? Not time type: %#v", rx)
		return
	}

	ses := sessionMap[rx.Session]

	if ses == nil {
		log.Printf("Ping packet sent without an active session by %s", rxAddr)
		return
	}
	if ses.UDPActivated == false && ses.TCPActivated {
		log.Printf("Ping packet sent but session is not double activated %s", rxAddr)
		return
	}

	pI := pingInfo{
		ID: rx.ID,
		TX: rx.TXTime,
		RX: timeRX,
	}
	ses.LastAcks[ses.getNextAckSlot()] = pI
	ses.ReplyWith = lSocket
	ses.ReplyTo = rxAddr
	ses.LastRX = timeRX
	ses.LastRXPing = rx

	if *debugFlagSlotShow {
		for n, v := range ses.LastAcks {
			fmt.Printf("\t[Slot %d] ID: %d - TX: %s\n", n, v.ID, v.TX.Sub(v.RX))
		}
	}

	if *debugShowLiveStats {
		RXL, TXL, RXLoss, TXLoss, exchanges := getStats(timeRX, rx, ses)
		log.Printf("[%s] RX: %s TX: %s [Loss RX: %d/%d | Loss TX %d/%d]", ses.PeerAddress, RXL, TXL, RXLoss, exchanges, TXLoss, exchanges)
	}

}

func handleInboundHandshake(buf []byte, rxAddr *net.UDPAddr, lSocket net.PacketConn) {

	rx := handshakeStruct{}
	err := msgpack.Unmarshal(buf, &rx)
	if err != nil {
		log.Printf("Failed to parse packet from %v", rxAddr.String())
		return
	}

	if rx.Magic != 11181 {
		log.Printf("Invalid magic from %v", rxAddr.String())
		return
	}

	if rx.Version != 3 {
		log.Printf("Invalid UDP handshake version from %v", rxAddr.String())
		return
	}

	ses := sessionMap[rx.Session]
	if ses == nil {
		log.Printf("Handshake packet sent without an active session by %s", rxAddr)
		return
	}

	if ses.UDPActivated && ses.TCPActivated {
		log.Printf("Handshake packet sent but session *is* double activated %s", rxAddr)
		return
	}
	wasAlreadyActivated := ses.UDPActivated

	// Well cool, Looks good, let's activate our end and send the same thing back to them
	ses.ReplyTo = rxAddr
	ses.ReplyWith = *globalReplyWith
	ses.UDPActivated = true
	if ses.PeerAddress == nil {
		ses.PeerAddress = rxAddr.IP
	}

	select {
	case ses.UDPHandshake <- true:
	default:
		log.Printf("Tried to activate socket, but failed to because activation notification pipe was full")
	}

	if !wasAlreadyActivated {
		ses.ReplyWith.WriteTo(buf, ses.ReplyTo)
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

	RXLoss, TXLoss, TotalSent = getLoss(rx, ses)

	return RXLatency, TXLatency, RXLoss, TXLoss, TotalSent
}

func getLoss(rx pingStruct, ses *session) (RXLoss int, TXLoss int, TotalSent int) {

	TipID := uint8(time.Now().Unix()%255) + 1

	// Don't send loss stats when we don't have enough info to operate with
	if dumbLastAckSearchForID(0, ses.LastAcks) {
		return 0, 0, 0
	}

	// Okay so we have enough data, let's search backwards (avoiding 0)
	Starting := TipID - 32
	i := Starting
	for {
		i++
		if i == 0 {
			i++
		}
		if i == TipID {
			break
		}
		if !dumbLastAckSearchForID(i, rx.LastAcks) {
			TXLoss++
		}
		if !dumbLastAckSearchForID(i, ses.LastAcks) {
			RXLoss++
		}
	}

	return RXLoss, TXLoss, 32
}

func dumbLastAckSearchForID(targetID uint8, LastAcks [32]pingInfo) bool {
	for _, v := range LastAcks {
		if v.ID == targetID {
			return true
		}
	}

	return false
}

type pingStruct struct {
	Type         uint8        `msgpack:"Y"` // MUST be 't' for a time sync
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
