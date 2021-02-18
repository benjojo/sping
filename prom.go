package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	listenAddress = flag.String("web.listen-address", "[::]:9523", "Address on which to expose metrics and web interface")
	metricsPath   = flag.String("web.telemetry-path", "/metrics", "Path under which to expose metrics.")
)

func handlePrometheus() {
	prometheus.MustRegister(Collector{})
	handler := promhttp.HandlerFor(prometheus.DefaultGatherer,
		promhttp.HandlerOpts{})

	http.Handle(*metricsPath, handler)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if *metricsPath == "/metrics" {
			w.Write([]byte(`<html>
			<head><title>split-ping</title></head>
			<body>
			<h1>split-ping</h1>
			<p><a href="` + *metricsPath + `">Metrics</a></p>
			</body>
			</html>`))
		} else {
			// Let's not expose a URL that has been custom set, just in case people are
			// trying security by obscurity
			w.Write([]byte(`<html>
			<head><title>split-ping</title></head>
			<body>
			<h1>split-ping</h1>
			</body>
			</html>`))
		}

	})

	log.Print("Listening on", *listenAddress)
	if err := http.ListenAndServe(*listenAddress, nil); err != nil {
		log.Fatal(err)
	}
}

//Collector implements the prometheus.Collector interface.
type Collector struct{}

//Describe implements the prometheus.Collector interface.
func (c Collector) Describe(ch chan<- *prometheus.Desc) {
	promLatency.Describe(ch)
	promLoss.Describe(ch)
}

//Collect implements the prometheus.Collector interface.
func (c Collector) Collect(ch chan<- prometheus.Metric) {
	err := c.measure()
	//only report data when measurement was successful
	if err == nil {
		promLatency.Collect(ch)
		promLoss.Collect(ch)
	} else {
		log.Println("ERROR:", err)
		return
	}
}

var (
	promLatency = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "splitping_latency",
			Help: "The latency (in s) in each direction",
		},
		[]string{"direction", "host"},
	)
	promLoss = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "splitping_loss",
			Help: "The loss in (in persent) each direction",
		},
		[]string{"direction", "host"},
	)
)

func (c Collector) measure() error {
	sessionLock.Lock()
	for _, v := range sessionMap {
		RXL, TXL, RXLoss, TXLoss, exchanges := getStats(v.LastRX, v.LastRXPing, v)
		PeerAddr := v.PeerAddress.String()
		promLatency.WithLabelValues("rx", PeerAddr).Set(float64(RXL.Seconds()))

		promLatency.WithLabelValues("tx", PeerAddr).Set(float64(TXL.Seconds()))
		if exchanges == 32 {
			promLoss.WithLabelValues("rx", PeerAddr).Set(float64(RXLoss) / 32)
			promLoss.WithLabelValues("tx", PeerAddr).Set(float64(TXLoss) / 32)
		}
	}
	sessionLock.Unlock()
	return nil
}
