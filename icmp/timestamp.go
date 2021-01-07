// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package icmp

import (
	"encoding/binary"
)

// An Timestamp represents an ICMP Timestamp request or reply message body.
type Timestamp struct {
	ID        int // identifier
	Seq       int // sequence number
	Originate int
	Receive   int
	Transmit  int
}

// Len implements the Len method of MessageBody interface.
func (p *Timestamp) Len(proto int) int {
	if p == nil {
		return 0
	}
	return 4 + (4 * 3)
}

// Marshal implements the Marshal method of MessageBody interface.
func (p *Timestamp) Marshal(proto int) ([]byte, error) {
	b := make([]byte, 4+(4*3))
	binary.BigEndian.PutUint16(b[:2], uint16(p.ID))
	binary.BigEndian.PutUint16(b[2:4], uint16(p.Seq))
	binary.BigEndian.PutUint32(b[4:8], uint32(p.Originate))
	binary.BigEndian.PutUint32(b[8:12], uint32(p.Receive))
	binary.BigEndian.PutUint32(b[12:12+4], uint32(p.Transmit))
	return b, nil
}

// parseEcho parses b as an ICMP echo request or reply message body.
func parseTimestamp(proto int, _ Type, b []byte) (MessageBody, error) {
	bodyLen := len(b)
	if bodyLen < 4+(4*3) {
		return nil, errMessageTooShort
	}
	p := &Timestamp{
		ID:        int(binary.BigEndian.Uint16(b[:2])),
		Seq:       int(binary.BigEndian.Uint16(b[2:4])),
		Originate: int(binary.BigEndian.Uint32(b[4:8])),
		Receive:   int(binary.BigEndian.Uint32(b[8:12])),
		Transmit:  int(binary.BigEndian.Uint32(b[12:16])),
	}
	return p, nil
}
