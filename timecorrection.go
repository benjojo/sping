package main

import (
	"log"
	"time"
)

var timeOffset time.Duration
var lastSync time.Time

func getTimeOffset() time.Duration {
	if lastSync.IsZero() || time.Since(lastSync) > time.Minute*30 {
		timeOffset = time.Duration(calibrateAgainstApple())

		if timeOffset.Seconds() > 1 {
			log.Fatalf("Time is too out of sync on system for this tool to be helpful, please run NTP on your system clock")
		}
	}

	return timeOffset
}

func timeNowCorrected() time.Time {
	return time.Now().Add(getTimeOffset())
}
