package main

import (
	probing "github.com/prometheus-community/pro-bing"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/push"
	"os"
)

func main() {
	registry := prometheus.NewRegistry()

	labels := prometheus.Labels{"client": os.Getenv("PINGPG_CLIENTID")}
	minRTT := prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "min_rtt_ms",
		ConstLabels: labels,
	})
	maxRTT := prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "max_rtt_ms",
		ConstLabels: labels,
	})
	avgRTT := prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "avg_rtt_ms",
		ConstLabels: labels,
	})
	stdDevRTT := prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "std_dev_rtt",
		ConstLabels: labels,
	})
	packetLoss := prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "packet_loss",
		ConstLabels: labels,
	})

	registry.MustRegister(minRTT, maxRTT, avgRTT, stdDevRTT, packetLoss)
	pusher := push.New("http://pingpg.keeys.io", "ping-pg").Gatherer(registry).BasicAuth("pingpg", os.Getenv("PINGPG_PASS"))

	pinger, err := probing.NewPinger("www.google.com")
	if err != nil {
		panic(err)
	}
	pinger.Count = 10

	for {
		err = pinger.Run()
		if err != nil {
			panic(err)
		}

		stats := pinger.Statistics()
		minRTT.Set(float64(stats.MinRtt.Milliseconds()))
		maxRTT.Set(float64(stats.MaxRtt.Milliseconds()))
		avgRTT.Set(float64(stats.AvgRtt.Milliseconds()))
		stdDevRTT.Set(float64(stats.StdDevRtt.Milliseconds()))
		packetLoss.Set(stats.PacketLoss)

		err := pusher.Push()
		if err != nil {
			println(err)
		}
	}

}
