package main

import (
	"flag"
	"log"
	"os"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

var ppsPath = flag.String("pps.path", "/dev/pps0", "what PPS device to use")
var usePPS = flag.Bool("use.pps", false, "If to use a PPS device instead of system clock")

func setupPPS() {
	f, err := os.OpenFile(*ppsPath, os.O_RDWR, 0)
	if err != nil {
		log.Fatalf("Unable to open pps device %#v", err)
	}

	FD := f.Fd()
	a := int(FD)
	ppsFD = &a
	ppsFile = f

	PP := PPSKParams{}
	unix.Syscall(unix.SYS_IOCTL, uintptr(*ppsFD), uintptr(PPS_GETPARAMS), uintptr(unsafe.Pointer(&PP)))
	log.Printf("PPS Cap: %#v", PP)
	PP.Mode = 0x01  // PPS_CAPTUREASSERT
	PP.Mode |= 0x10 // PPS_OFFSETASSERT
	PP.Assert_off_tu.Nsec = 0
	PP.Assert_off_tu.Sec = 0
	_, _, err2 := unix.Syscall(unix.SYS_IOCTL, uintptr(*ppsFD), uintptr(PPS_SETPARAMS), uintptr(unsafe.Pointer(&PP)))
	log.Printf("PPS Set Cap: %#v", PP)
	if err2 != 0 {
		log.Fatalf("Failed to setup pps device %v", err)
	}
}

var ppsFD *int
var ppsFile *os.File // To stop GC
var ppsDebug = flag.Bool("pps.debug", false, "Enable debug output for PPS inputs")

func waitForPPSPulse() time.Time {
	if ppsFD == nil {
		setupPPS()
	}

	a := PPSFData{}
	// a.Timeout.Sec = time.Now().Unix() + 2
	a.Timeout.Sec = 3
	_, _, err := unix.Syscall(unix.SYS_IOCTL, uintptr(*ppsFD), uintptr(PPS_FETCH), uintptr(unsafe.Pointer(&a)))
	if err != 0 {
		log.Printf("PPS Pulse failed! %v / FD %v", err, *ppsFD)
	}
	if *ppsDebug {
		log.Printf("%#v", a)
	}
	return time.Unix(a.Info.Assert_tu.Sec, int64(a.Info.Assert_tu.Nsec))
}

type PPSKInfo struct {
	Assert_sequence uint32
	Clear_sequence  uint32
	Assert_tu       PPSKTime
	Clear_tu        PPSKTime
	Current_mode    int32
	_               [4]byte
}

type PPSFData struct {
	Info    PPSKInfo
	Timeout PPSKTime
}

type PPSKParams struct {
	Api_version   int32
	Mode          int32
	Assert_off_tu PPSKTime
	Clear_off_tu  PPSKTime
}

type PPSKTime struct {
	Sec   int64
	Nsec  int32
	Flags uint32
}
