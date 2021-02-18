package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"github.com/benjojo/sping/icmp"
	"golang.org/x/net/ipv4"
)

// Default to listen on all IPv4 interfaces
var ListenAddr = "0.0.0.0"

func Ping(addr string, seq int) (_ *net.IPAddr, _ time.Duration, _ *icmp.Timestamp, _ error) {
	// Start listening for icmp replies
	c, err := icmp.ListenPacket("ip4:icmp", ListenAddr)
	if err != nil {
		return nil, 0, nil, err
	}
	defer c.Close()

	// Resolve any DNS (if used) and get the real IP of the target
	dst, err := net.ResolveIPAddr("ip4", addr)
	if err != nil {
		return nil, 0, nil, err
	}
	// get the time
	now := time.Now().UTC()
	offset := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	// Make a new ICMP message
	m := icmp.Message{
		Type: ipv4.ICMPTypeTimestamp, Code: 0,
		Body: &icmp.Timestamp{
			ID: os.Getpid() & 0xffff, Seq: seq, //<< uint(seq), // TODO
			Originate: int(now.Sub(offset) / 1000000),
		},
	}
	b, err := m.Marshal(nil)
	if err != nil {
		return dst, 0, nil, err
	}

	// Send it
	start := time.Now()
	n, err := c.WriteTo(b, dst)
	if err != nil {
		return dst, 0, nil, err
	} else if n != len(b) {
		return dst, 0, nil, fmt.Errorf("got %v; want %v", n, len(b))
	}

	// Wait for a reply
	reply := make([]byte, 1500)
	err = c.SetReadDeadline(time.Now().Add(10 * time.Second))
	if err != nil {
		return dst, 0, nil, err
	}
	n, peer, err := c.ReadFrom(reply)
	if err != nil {
		return dst, 0, nil, err
	}
	duration := time.Since(start)

	// Pack it up boys, we're done here
	rm, err := icmp.ParseMessage(1, reply[:n])
	if err != nil {
		return dst, 0, nil, err
	}
	switch rm.Type {
	case ipv4.ICMPTypeEchoReply:
		return dst, duration, nil, nil
	case ipv4.ICMPTypeTimestampReply:
		var body *icmp.Timestamp
		body = rm.Body.(*icmp.Timestamp)
		return dst, duration, body, nil
	default:
		return dst, 0, nil, fmt.Errorf("got %+v from %v; want echo reply", rm, peer)
	}
}

func main() {
	p := func(addr string) {
		dst, dur, ts, err := Ping(addr, 1)
		if err != nil {
			log.Printf("Ping %s (%s): %s\n", addr, dst, err)
			return
		} else {

			fmt.Printf("TS %s (%s): Forward: %dms Back: %dms RTT(%dms)\n",
				addr, dst, ts.Transmit-ts.Originate, int(dur.Milliseconds())-(ts.Transmit-ts.Originate), dur.Milliseconds())

		}
	}
	flag.Usage = func() {
		fmt.Print(`icmp-timestap-pinger <hosts>

No flags, Will send a ICMP Timestamp request to hosts and estimate forward and back latency,
Can only work correctly if both the host and client have near perfectly syncd clocks.
`)
		os.Exit(1)
	}
	flag.Parse()
	hosts := flag.Args()
	if len(hosts) == 0 {
		flag.Usage()
	}

	for _, v := range hosts {
		p(v)
	}

	// p("reddit.com")
	// p("klir.benjojo.co.uk")
	// p("airmail.benjojo.co.uk")
	// p("flux.basil.pw")
	// p("syd-au-ping.vultr.com")

}
