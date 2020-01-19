package main

import (
	"flag"
	"fmt"
	"log"
	"net"
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
	flag.Parse()

	packetLimiter = rate.NewLimiter(rate.Limit(*udpPPSin), (*udpPPSin)*3)
	getTimeOffset()

	go listenAndRoute()
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
}

var sessionMap map[uint32]session

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

		go handlePacket(buf[n:], rxAddr)
	}
}

func handlePacket(buf []byte, rxAddr net.Addr) {
	timeRX := timeNowCorrected()

	rx := pingStruct{}
	err := msgpack.Unmarshal(buf, &rx)
	if err != nil {
		return
	}

	if rx.Magic != 11181 {
		return
	}

	if !sessionMap[rx.Session].Init {
		return
	}

	fmt.Printf("[%v] %#v\n", rxAddr.String(), rx)

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
