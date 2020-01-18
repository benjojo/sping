package main

import (
	"fmt"
	"log"
	"time"
)

/*
 [+] Auth packet
	[+] Respond with cookie to use
	[+] If failed, just don't repsond
 [+] Ping packets
*/

var timeOffset time.Duration

func main() {
	timeOffset = time.Duration(calibrateAgainstApple())

	if timeOffset.Seconds() > 1 {
		log.Fatalf("Time is too out of sync on system for this tool to be helpful, please run NTP on your system clock")
	}

	for {
		a := timeNowCorreted().Unix()
		u := time.Until(time.Unix(a+1, 0).Add(timeOffset * -1))
		time.Sleep(u)
		fmt.Printf("it is now: %s\n", time.Now())
	}
}

func timeNowCorreted() time.Time {
	return time.Now().Add(timeOffset)
}

type pingStruct struct {
	Magic         uint16       `msgpack:"M"`
	Session       uint32       `msgpack:"S"`
	ID            uint8        `msgpack:"I"`
	TXUnixSeconds uint32       `msgpack:"T"`
	LastAcks      [32]pingInfo `msgpack:"A"`
}

type pingInfo struct {
	ID             uint8  `msgpack:"R"`
	DifferenceOnRX uint32 `msgpack:"X"` // Microseconds
}
