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
// TODO: switch to OTEL remote write

func main() {
	wrb := make(chan *prompb.WriteRequest, 100)
	go func() {
		for {
			select {
			case wr := <-wrb:
				publishWithRetry(wr)
			}
		}
	}()

	var l []prompb.Label
	for {
		s, err := probe()
		if err != nil {
			println(err.Error())
			s = &probing.Statistics{
				PacketLoss: 100,
			}
		}

		nl, err := updateLabels(l)
		if err != nil {
			println(err.Error())
		} else {
			l = nl
		}

		wr := statisticsToWriteRequest(s, l)
		wrb <- wr

		time.Sleep(time.Second * 10)
	}
}

func updateLabels(labels []prompb.Label) ([]prompb.Label, error) {
	return []prompb.Label{
		{
			Name:  "client_id",
			Value: os.Getenv("PINGPG_CLIENTID"),
		},
	}, nil
}

func publishWithRetry(wr *prompb.WriteRequest) {
	err := publish(wr)
	if err != nil {
		println(err.Error())
		time.Sleep(time.Second * 2)
		publishWithRetry(wr)
	}
}

func publish(wr *prompb.WriteRequest) error {
	pb, err := proto.Marshal(wr)
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

func statisticsToWriteRequest(s *probing.Statistics, l []prompb.Label) *prompb.WriteRequest {
	now := timestamp.FromTime(time.Now().UTC())
	wr := &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{
			{
				Labels: []prompb.Label{
					{
						Name:  "__name__",
						Value: "min_rtt_ns",
					},
				},
				Samples: []prompb.Sample{
					{
						Value:     float64(s.MinRtt.Nanoseconds()),
						Timestamp: now,
					},
				},
			},
			{
				Labels: []prompb.Label{
					{
						Name:  "__name__",
						Value: "max_rtt_ns",
					},
				},
				Samples: []prompb.Sample{
					{
						Value:     float64(s.MaxRtt.Nanoseconds()),
						Timestamp: now,
					},
				},
			},
			{
				Labels: []prompb.Label{
					{
						Name:  "__name__",
						Value: "avg_rtt_ns",
					},
				},
				Samples: []prompb.Sample{
					{
						Value:     float64(s.AvgRtt.Nanoseconds()),
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
				},
				Samples: []prompb.Sample{
					{
						Value:     float64(s.StdDevRtt.Nanoseconds()),
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
				},
				Samples: []prompb.Sample{
					{
						Value:     s.PacketLoss,
						Timestamp: now,
					},
				},
			},
		},
	}

	for i, ts := range wr.Timeseries {
		wr.Timeseries[i].Labels = append(ts.Labels, l...)
	}

	return wr
}

func probe() (*probing.Statistics, error) {
	pinger, err := probing.NewPinger("1.1.1.1")
	if err != nil {
		return nil, err
	}
	pinger.Count = 10
	pinger.SetPrivileged(true)

	err = pinger.Run()
	if err != nil {
		return nil, err
	}

	return pinger.Statistics(), nil
}
