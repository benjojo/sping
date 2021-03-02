package main

import (
	"flag"
)

var ppsPath = flag.String("pps.path", "/dev/pps0", "what PPS device to use")
var usePPS = flag.Bool("use.pps", false, "If to use a PPS device instead of system clock")
