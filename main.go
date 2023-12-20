package main

import (
	probing "github.com/prometheus-community/pro-bing"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/push"
	"os"
	"time"
)

// TODO: discovery local gateway and publish ping metrics for it

func main() {
	registry := prometheus.NewRegistry()

	labels := prometheus.Labels{}
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
		Name:        "std_dev_rtt_ns",
		ConstLabels: labels,
	})
	packetLoss := prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        "packet_loss",
		ConstLabels: labels,
	})

	registry.MustRegister(minRTT, maxRTT, avgRTT, stdDevRTT, packetLoss)
	pusher := push.New("http://pingpg.keeys.io", os.Getenv("PINGPG_CLIENTID")).Gatherer(registry).BasicAuth("pingpg", os.Getenv("PINGPG_PASS"))

	for {
		pinger, err := probing.NewPinger("www.google.com")
		if err != nil {
			println(err.Error())
		}
		pinger.Count = 10
		pinger.SetPrivileged(true)

		err = pinger.Run()
		if err != nil {
			println(err.Error())
		}

		stats := pinger.Statistics()
		minRTT.Set(float64(stats.MinRtt.Milliseconds()))
		maxRTT.Set(float64(stats.MaxRtt.Milliseconds()))
		avgRTT.Set(float64(stats.AvgRtt.Milliseconds()))
		stdDevRTT.Set(float64(stats.StdDevRtt.Nanoseconds()))
		packetLoss.Set(stats.PacketLoss)

		err = pusher.Push()
		if err != nil {
			println(err.Error())
		}

		time.Sleep(time.Second * 10)
	}

}
