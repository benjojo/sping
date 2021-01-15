package main

import (
	"crypto/rand"
	"encoding/binary"
	"log"
	"net"

	"github.com/vmihailenco/msgpack/v4"
)

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
	Magic   uint16 `msgpack:"MHand"`
	Version uint16 `msgpack:"Ver"`
	Session uint32 `msgpack:"Ses"`
}

func handleTCPconnection(conn net.Conn) {
	defer conn.Close()

	_, err := conn.Write([]byte("sping-0.2-https://github.com/benjojo/sping\n"))
	if err != nil {
		return
	}
	buf := make([]byte, 10000)
	n, err := conn.Read(buf)
	if n > 9000 {
		// Responses that big are bogus, and can be nuked
		return
	}

	handshake := handshakeStruct{}

	err = msgpack.Unmarshal(buf, &handshake)
	if err != nil {
		return
	}

	if handshake.Magic != 11181 {
		return
	}

	if handshake.Version != 2 {
		log.Printf("Unsupported handshake (%#v) on %#v", handshake.Version, conn.RemoteAddr().String())
		return
	}

	nSes := newSessionID()

	returnPacket := handshakeStruct{
		Magic:   11181,
		Version: 2,
		Session: nSes,
	}

	sessionLock.Lock()
	sessionMap[nSes] = &session{
		Init:        true,
		PeerAddress: conn.RemoteAddr().(*net.TCPAddr).IP,
		SessionID:   nSes,
	}
	sessionLock.Unlock()
	go sessionMap[nSes].sendPackets()

	b, err := msgpack.Marshal(returnPacket)
	if err == nil {
		conn.Write(b)
	}
}

func newSessionID() uint32 {
	o := uint32(0)
	binary.Read(rand.Reader, binary.BigEndian, &o)
	return o
}
