package main

import (
	"bytes"
	"context"
	"github.com/golang/protobuf/proto"
	"github.com/golang/snappy"
	probing "github.com/prometheus-community/pro-bing"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/prompb"

	"net/http"
	"os"
	"time"
)

// TODO: discovery local gateway and publish ping metrics for it
// TODO: detect wifi / eth
// TODO: retry, publish historical metrics
// TODO: impl using prom remote write
// TODO: switch to OTEL remote write

func main() {
	for {
		err := collectAndPush()
		if err != nil {
			println(err.Error())
		}
		time.Sleep(time.Second * 10)
	}
}

func collectAndPush() error {
	pinger, err := probing.NewPinger("1.1.1.1")
	if err != nil {
		return err
	}
	pinger.Count = 10
	pinger.SetPrivileged(true)

	err = pinger.Run()
	if err != nil {
		return err
	}

	now := timestamp.FromTime(time.Now().UTC())
	var r = prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{
						Name:  "__name__",
						Value: "min_rtt_ms",
					},
					{
						Name:  "exported_job",
						Value: os.Getenv("PINGPG_CLIENTID"),
					},
				},
				Samples: []prompb.Sample{
					{
						Value:     float64(pinger.Statistics().MinRtt.Milliseconds()),
						Timestamp: now,
					},
				},
			},
			{
				Labels: []prompb.Label{
					{
						Name:  "__name__",
						Value: "max_rtt_ms",
					},
					{
						Name:  "exported_job",
						Value: os.Getenv("PINGPG_CLIENTID"),
					},
				},
				Samples: []prompb.Sample{
					{
						Value:     float64(pinger.Statistics().MaxRtt.Milliseconds()),
						Timestamp: now,
					},
				},
			},
			{
				Labels: []prompb.Label{
					{
						Name:  "__name__",
						Value: "avg_rtt_ms",
					},
					{
						Name:  "exported_job",
						Value: os.Getenv("PINGPG_CLIENTID"),
					},
				},
				Samples: []prompb.Sample{
					{
						Value:     float64(pinger.Statistics().AvgRtt.Milliseconds()),
						Timestamp: now,
					},
				},
			},
			{
				Labels: []prompb.Label{
					{
						Name:  "__name__",
						Value: "std_dev_rtt_ns",
					},
					{
						Name:  "exported_job",
						Value: os.Getenv("PINGPG_CLIENTID"),
					},
				},
				Samples: []prompb.Sample{
					{
						Value:     float64(pinger.Statistics().StdDevRtt.Milliseconds()),
						Timestamp: now,
					},
				},
			},
			{
				Labels: []prompb.Label{
					{
						Name:  "__name__",
						Value: "packet_loss",
					},
					{
						Name:  "exported_job",
						Value: os.Getenv("PINGPG_CLIENTID"),
					},
				},
				Samples: []prompb.Sample{
					{
						Value:     pinger.Statistics().PacketLoss,
						Timestamp: now,
					},
				},
			},
		},
	}
	pb, err := proto.Marshal(&r)
	if err != nil {
		return err
	}

	cb := snappy.Encode(nil, pb)
	t, _ := context.WithTimeout(context.Background(), time.Second*3)
	req, err := http.NewRequestWithContext(t, http.MethodPost, "http://pingpg.keeys.io/api/v1/write", bytes.NewBuffer(cb))
	if err != nil {
		return err
	}
	req.Header.Add("Content-Encoding", "snappy")
	req.Header.Set("Content-Type", "application/x-protobuf")
	req.Header.Add("User-Agent", "pingpg-v0.0.1")
	req.Header.Add("X-Prometheus-Remote-Write-Version", "0.1.0")
	req.SetBasicAuth("pingpg", os.Getenv("PINGPG_PASS"))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	println(resp.Status)
	return nil
}
