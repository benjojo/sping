split-ping
===

split-ping is a tool that can tell you what direction packet latency or loss is on. This is handy for network debugging and locating congestion.

It was built as part of a personal blog post: https://blog.benjojo.co.uk/post/ping-with-loss-latency-split

split-ping's binary name is shorted to sping for ease, it's help page is as follows:

```
$ ./sping -h
Usage of ./sping:
  -clock-is-perfect
        Enable userspace calibration against Apple's GPS NTP servers (default true)
  -debug.showslots
        Show incoming packet latency slots
  -debug.showstats
        Show per ping info, and timestamps
  -listenAddr string
        Listening address (default "[::]:6924")
  -peers string
        List of IPs that are peers
  -pps.debug
        Enable debug output for PPS inputs
  -pps.path string
        what PPS device to use (default "/dev/pps0")
  -udp.pps int
        max inbound PPS that can be processed at once (default 100)
  -use.pps
        If to use a PPS device instead of system clock
  -web.listen-address string
        Address on which to expose metrics and web interface (default "[::]:9523")
  -web.telemetry-path string
        Path under which to expose metrics. (default "/metrics")
```

## Example output

When viewing the stats via the CLI:
```bash
# run this on each host, pointed at the other host as a peer
$ ./sping -peers 198.16.109.36 -debug.showstats
2021/03/03 19:04:10 Listening on[::]:9523
it is now: 2021-03-03 19:04:11.000241999 +0000 GMT m=+0.500559754
it is now: 2021-03-03 19:04:12.000174134 +0000 GMT m=+1.500491809
2021/03/03 19:04:12 [198.16.109.36] RX: 8.131386ms TX: 0s [Loss RX: 0/0 | Loss TX 0/0]
...
```

When viewing the stats via the web interface:
```bash
$ ./sping -peers 198.16.109.36
$ open http://127.0.0.1:9523/metrics
```
```logs
# HELP splitping_latency The latency (in seconds) in each direction
# TYPE splitping_latency gauge
splitping_latency{direction="rx",host="23.132.96.179"} 0.068701256
splitping_latency{direction="tx",host="23.132.96.179"} 0.066165156
# HELP splitping_loss The loss in each direction
# TYPE splitping_loss gauge
splitping_loss{direction="rx",host="23.132.96.179"} 0
splitping_loss{direction="tx",host="23.132.96.179"} 0
...
```

## Building

A simple `go build` in this directory should build sping (after auto-fetching the go modules)
