package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"sort"
	"sync"
	"time"

	"github.com/beevik/ntp"
)

/*
Apple servers discovered using (in bash):

declare -a arr=("sgsin3" "brsao4" "hkhkg1" "hkhkg1" "ussjc2" "uslax1" "usnyc3" "ausyd2" "usqas2" "frcch1" "uklon5" "usmia1" "usatl4" "nlams2" "jptyo5" "usscz2" "sesto4" "defra1" "usdal2" "uschi5" "twtpe2" "krsel6" )
for zone in "${arr[@]}"
do
	for i in {1..3}
	do
		A=$(dig +short "$zone-ntp-00$i.aaplimg.com")
		printf '"'
		printf "$A\", //"
		echo "$zone-ntp-00$i.aaplimg.com"
	done
done
*/
var ntpServers = []string{
	"17.253.82.125",  //sgsin3-ntp-001.aaplimg.com
	"17.253.82.253",  //sgsin3-ntp-002.aaplimg.com
	"17.253.18.125",  //brsao4-ntp-001.aaplimg.com
	"17.253.18.253",  //brsao4-ntp-002.aaplimg.com
	"17.253.84.125",  //hkhkg1-ntp-001.aaplimg.com
	"17.253.84.253",  //hkhkg1-ntp-002.aaplimg.com
	"17.253.84.123",  //hkhkg1-ntp-003.aaplimg.com
	"17.253.4.125",   //ussjc2-ntp-001.aaplimg.com
	"17.253.4.253",   //ussjc2-ntp-002.aaplimg.com
	"17.253.26.125",  //uslax1-ntp-001.aaplimg.com
	"17.253.26.253",  //uslax1-ntp-002.aaplimg.com
	"17.253.14.125",  //usnyc3-ntp-001.aaplimg.com
	"17.253.14.253",  //usnyc3-ntp-002.aaplimg.com
	"17.253.14.123",  //usnyc3-ntp-003.aaplimg.com
	"17.253.66.125",  //ausyd2-ntp-001.aaplimg.com
	"17.253.66.253",  //ausyd2-ntp-002.aaplimg.com
	"17.253.20.125",  //usqas2-ntp-001.aaplimg.com
	"17.253.20.253",  //usqas2-ntp-002.aaplimg.com
	"17.253.108.125", //frcch1-ntp-001.aaplimg.com
	"17.253.108.253", //frcch1-ntp-002.aaplimg.com
	"17.253.34.125",  //uklon5-ntp-001.aaplimg.com
	"17.253.34.253",  //uklon5-ntp-002.aaplimg.com
	"17.253.34.123",  //uklon5-ntp-003.aaplimg.com
	"17.253.12.125",  //usmia1-ntp-001.aaplimg.com
	"17.253.12.253",  //usmia1-ntp-002.aaplimg.com
	"17.253.6.125",   //usatl4-ntp-001.aaplimg.com
	"17.253.6.253",   //usatl4-ntp-002.aaplimg.com
	"17.253.52.125",  //nlams2-ntp-001.aaplimg.com
	"17.253.52.253",  //nlams2-ntp-002.aaplimg.com
	"17.253.68.125",  //jptyo5-ntp-001.aaplimg.com
	"17.253.68.253",  //jptyo5-ntp-002.aaplimg.com
	"17.253.68.123",  //jptyo5-ntp-003.aaplimg.com
	"17.253.16.125",  //usscz2-ntp-001.aaplimg.com
	"17.253.16.253",  //usscz2-ntp-002.aaplimg.com
	"17.253.38.125",  //sesto4-ntp-001.aaplimg.com
	"17.253.38.253",  //sesto4-ntp-002.aaplimg.com
	"17.253.54.125",  //defra1-ntp-001.aaplimg.com
	"17.253.54.253",  //defra1-ntp-002.aaplimg.com
	"17.253.54.123",  //defra1-ntp-003.aaplimg.com
	"17.253.2.125",   //usdal2-ntp-001.aaplimg.com
	"17.253.2.253",   //usdal2-ntp-002.aaplimg.com
	"17.253.24.125",  //uschi5-ntp-001.aaplimg.com
	"17.253.24.253",  //uschi5-ntp-002.aaplimg.com
	"17.253.116.125", //twtpe2-ntp-001.aaplimg.com
	"17.253.116.253", //twtpe2-ntp-002.aaplimg.com
	"17.253.114.125", //krsel6-ntp-001.aaplimg.com
	"17.253.114.253", //krsel6-ntp-002.aaplimg.com
}

type ntpResult struct {
	RTT    time.Duration
	Offset time.Duration
	Host   string
}

type timeSyncs struct {
	mu sync.Mutex
	m  map[string]ntpResult
}

type byNTPRTT []ntpResult

func (s byNTPRTT) Len() int {
	return len(s)
}
func (s byNTPRTT) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
func (s byNTPRTT) Less(i, j int) bool {
	return s[i].RTT < s[j].RTT
}

type byNTPOffset []ntpResult

func (s byNTPOffset) Len() int {
	return len(s)
}
func (s byNTPOffset) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
func (s byNTPOffset) Less(i, j int) bool {
	return s[i].Offset < s[j].Offset
}

var flagClockIsPerfect = flag.Bool("clock-is-perfect", true, "Enable userspace calibration against Apple's GPS NTP servers")

// Returns out offset against apple's NTP (+ GPS) servers
func calibrateAgainstApple() int {
	if *flagClockIsPerfect {
		lastSync = time.Now()
		return 1
	}

	ts := timeSyncs{m: make(map[string]ntpResult)}

	log.Printf("Calibrating myself against Apple")
	for _, v := range ntpServers {
		go func(server string) {
			offset, RTT, failed := ntpTriplePoll(server)
			if failed > 1 || RTT == 0 || offset == 0 {
				return
			}
			ts.mu.Lock()
			ts.m[server] = ntpResult{
				RTT:    RTT,
				Offset: offset,
				Host:   server,
			}
			ts.mu.Unlock()
		}(v)

		time.Sleep(time.Millisecond * time.Duration(10+rand.Intn(50)))
	}
	time.Sleep(time.Second * 4)

	// now we have a NTP resp from all of them, let's figure out how out of sync we are with GPS
	log.Printf("-----------------------------------------")

	NTPresArray := make([]ntpResult, 0)
	for _, v := range ts.m {
		NTPresArray = append(NTPresArray, v)
	}

	sort.Sort(byNTPRTT(NTPresArray))
	considerableNTPresponces := make([]ntpResult, 5)

	for count, Result := range NTPresArray {
		fmt.Printf("[%s] Offset: %v\t\tRTT: %v\n", Result.Host, Result.Offset, Result.RTT)
		considerableNTPresponces[count] = Result
		if count == 4 {
			break
		}
	}
	sort.Sort(byNTPOffset(considerableNTPresponces))
	log.Printf("So I think the clock offset it %v, based on sorting:", considerableNTPresponces[2].Offset)

	for count, Result := range considerableNTPresponces {
		fmt.Printf("[%s] Offset: %v\t\tRTT: %v\n", Result.Host, Result.Offset, Result.RTT)
		if count == 4 {
			break
		}
	}

	lastSync = time.Now()
	return int(considerableNTPresponces[2].Offset)
}

func ntpTriplePoll(server string) (Offset time.Duration, RTT time.Duration, failed int) {
	offsets := make([]int, 3)
	rtt := make([]int, 3)
	for i := 0; i < 2; i++ {
		ntpRes, err := ntp.QueryWithOptions(server, ntp.QueryOptions{
			Timeout: time.Second,
		})
		if err != nil {
			log.Printf("Failed to reach Apple NTP server %v", err)
			failed++
			continue
		}

		offsets[i] = int(ntpRes.ClockOffset)
		rtt[i] = int(ntpRes.RTT)
		time.Sleep(time.Millisecond * time.Duration(10+rand.Intn(50)))
	}

	sort.Ints(offsets[:])
	sort.Ints(rtt[:])

	return time.Duration(offsets[1]), time.Duration(rtt[1]), failed
}
