icmp-timestamp-pinger
===

Usage:

```
$ ./icmp-timestap-pinger
icmp-timestap-pinger <hosts>

No flags, Will send a ICMP Timestamp request to hosts and estimate forward and back latency,
Can only work correctly if both the host and client have near perfectly syncd clocks.
```

Normal Operation:

```
$ sudo ./icmp-timestap-pinger bbc.co.uk reddit.com scanme.nmap.org
TS bbc.co.uk (151.101.192.81): Forward: 1ms Back: 1ms RTT(2ms)
TS reddit.com (151.101.129.140): Forward: 2ms Back: 1ms RTT(3ms)
TS scanme.nmap.org (45.33.32.156): Forward: 73ms Back: 70ms RTT(143ms)
```

Building:

`go build`