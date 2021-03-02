// +build !linux

package main

import "log"

func setupPPS() {
	log.Fatalf("PPS input is not supported on this platform.")
}

func waitForPPSPulse() {
	log.Fatalf("PPS input is not supported on this platform.")
}
