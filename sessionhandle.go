package main

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/vmihailenco/msgpack/v4"
)

func startSession(ip net.IP) {
	first := true
	for {
		if !first {
			time.Sleep(time.Second)
		}
		first = false

		conn, err := net.Dial("tcp", net.JoinHostPort(ip.String(), "6924"))
		if err != nil {
			log.Printf("Cannot (TCP) handshake to %v: %v", ip, err)
			continue
		}

		bannerBuf := make([]byte, 10000)
		n, err := conn.Read(bannerBuf)
		if n > 9000 {
			log.Printf("%v: Host banner too big", ip.String())
			conn.Close()
			continue
		}

		if !strings.HasPrefix(string(bannerBuf[:n]), "sping-0.3-") {
			log.Printf("%v: Host banner not sping", ip.String())
			conn.Close()
			continue
		}

		// defer conn.Close()
		// [+] Send Session Starting Request
		_, err = conn.Write([]byte("INVITE\r\n"))
		if err != nil {
			log.Printf("%v: Failed to ask for invite", ip.String())
			conn.Close()
			continue
		}
		// [+] Read the Invite Banner
		inviteBuf := make([]byte, 10)
		n, err = conn.Read(inviteBuf)
		if n > 31 || n == 0 {
			log.Printf("%v: Invite banner wrong size %d", ip.String(), n)
			conn.Close()
			continue
		}

		conn.Close()
		invite, err := strconv.ParseUint(string(inviteBuf[:n]), 10, 32)
		if err != nil {
			log.Printf("%v: Invite session bad", ip.String())
			continue
		}

		// [+] Make the internal session with the invite banner
		// [+] Put the session in the session table, Flagged as TCP handshaked
		sessionLock.Lock()
		sessionMap[uint32(invite)] = &session{
			PeerAddress:  ip,
			SessionID:    uint32(invite),
			TCPActivated: true,
			MadeByMe:     true,
			SessionMade:  time.Now(),
			UDPHandshake: make(chan bool, 0),
		}
		sessionLock.Unlock()
		// [+] Start the UDP Handshaker
		go sessionMap[uint32(invite)].sendUDPHandshake()
		// [+] Monitor the session table for the session disappearing and restart session if gone
		for {
			time.Sleep(time.Second * 10)
			sessionLock.Lock()
			SessionExists := sessionMap[uint32(invite)] != nil
			sessionLock.Unlock()

			if SessionExists {
				continue
			} else {
				break
			}
		}
	}
}

// sendUDPHandshake is called by session.startSession to send UDP handshakes in the background
// the actual handshake is RX'd elsewhere, decoded and this function is notified of a sucessful
// handshake via a channel boop. At that point the function kicks off the actual send loop.
func (s *session) sendUDPHandshake() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	s.ReplyWith = *globalReplyWith

	for {
		select {
		case <-ticker.C:
			// Send a packet, but check if we have not already expired
			if time.Since(s.SessionMade) > time.Minute && !s.UDPActivated {
				// Clearly what we are doing is not working, time to stop
				log.Printf("Timed out UDP handshaking with %s", s.PeerAddress)
				return
			}
			// Cool no time out, let's send a handshake
			hs := handshakeStruct{
				Type:    'h',
				Magic:   11181,
				Session: s.SessionID,
				Version: 3,
			}
			b, err := msgpack.Marshal(hs)
			if err != nil {
				log.Fatalf("Failed to marshal packet %v / %#v", err, hs)
			}

			ua := net.UDPAddr{
				IP:   s.PeerAddress,
				Port: 6924,
			}

			s.ReplyWith.WriteTo(b, &ua)
		case <-s.UDPHandshake:
			// Okay cool, we god an ack, we can start sending packets
			s.UDPActivated = true
			go s.sendPackets()
			return
		}
	}
}

func listenOnTCP() {
	tListener, err := net.Listen("tcp", *bindAddr)
	if err != nil {
		log.Fatalf("Failed to listen on TCP port %v", err)
	}

	for {
		conn, err := tListener.Accept()
		if err != nil {
			log.Printf("Failed to accept connection, %v", err)
			continue
		}
		go handleTCPconnection(conn)
	}
}

type handshakeStruct struct {
	Type    uint8  `msgpack:"Y"` // MUST be 'h' for a handshake
	Magic   uint16 `msgpack:"M"`
	Version uint16 `msgpack:"V"`
	Session uint32 `msgpack:"S"`
}

func handleTCPconnection(conn net.Conn) {
	defer conn.Close()

	_, err := conn.Write([]byte("sping-0.3-https://github.com/benjojo/sping\n"))
	if err != nil {
		return
	}
	buf := make([]byte, 10000)
	n, err := conn.Read(buf)
	if n > 9000 {
		// Responses that big are bogus, and can be nuked
		return
	}

	if string(buf[:n]) != "INVITE\r\n" {
		conn.Write([]byte("I_DONT_UNDERSTAND"))
		return
	}

	nSes := newSessionID()

	sessionLock.Lock()
	sessionMap[nSes] = &session{
		SessionID:    nSes,
		MadeByMe:     false,
		TCPActivated: true,
		SessionMade:  time.Now(),
		UDPHandshake: make(chan bool, 0),
	}
	sessionLock.Unlock()
	go sessionMap[nSes].waitForHandshake()

	conn.Write([]byte(fmt.Sprint(nSes)))
}

func (s *session) waitForHandshake() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Send a packet, but check if we have not already expired
			if time.Since(s.SessionMade) > time.Minute && !s.UDPActivated {
				// Clearly what we are doing is not working, time to stop
				log.Printf("Timed out UDP handshaking with %s", s.PeerAddress)
				return
			}
		case <-s.UDPHandshake:
			// Okay cool, we god an ack, we can start sending packets
			s.UDPActivated = true
			go s.sendPackets()
			return
		}
	}
}

func newSessionID() uint32 {
	o := uint32(0)
	binary.Read(rand.Reader, binary.BigEndian, &o)
	return o
}
