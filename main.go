package main

import (
	"fmt"
	"log"
	"sort"
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

type NTPResult struct {
	NTPr   *ntp.Response
	RxTime time.Time
	Host   string
}

var timeSyncs map[string]NTPResult

type byNTPRTT []NTPResult

func (s byNTPRTT) Len() int {
	return len(s)
}
func (s byNTPRTT) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
func (s byNTPRTT) Less(i, j int) bool {
	return s[i].NTPr.RTT < s[j].NTPr.RTT
}

/*
 [+] Auth packet
	[+] Respond with cookie to use
	[+] If failed, just don't repsond
 [+] Ping packets
*/

func main() {
	timeSyncs = make(map[string]NTPResult)

	log.Printf("Calibrating myself against Apple")
	for _, v := range ntpServers {
		ntpRes, err := ntp.Query(v)
		if err != nil {
			log.Printf("Failed to reach Apple NTP server")
			continue
		}

		timeSyncs[v] = NTPResult{
			NTPr:   ntpRes,
			RxTime: time.Now(),
			Host:   v,
		}

		log.Printf("%#v lol\n", *ntpRes)
	}

	// now we have a NTP resp from all of them, let's figure out how out of sync we are with GPS
	log.Printf("-----------------------------------------")

	NTPresArray := make([]NTPResult, 0)
	for _, v := range timeSyncs {
		NTPresArray = append(NTPresArray, v)
	}

	sort.Sort(byNTPRTT(NTPresArray))

	for _, res := range NTPresArray {
		log.Printf("[%s] = CO: %v\tRTT: %v", res.Host, res.NTPr.ClockOffset, res.NTPr.RTT)
	}

	for {
		a := time.Now().Unix()
		u := time.Until(time.Unix(a+1, 0))
		time.Sleep(u)

		fmt.Printf("it is now: %s\n", time.Now())
	}
}
