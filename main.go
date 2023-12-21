package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/golang/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/net-byte/go-gateway"
	"github.com/pkg/errors"
	probing "github.com/prometheus-community/pro-bing"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/prompb"
	"io"
	"net/http/httputil"
	"strconv"
	"sync"

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

	l, err := getLabels()
	if err != nil {
		panic(err)
	}

	var lm sync.Mutex
	ticker := time.NewTicker(60 * time.Second)
	go func() {
		for range ticker.C {
			nl, err := getLabels()
			if err != nil {
				println(err)
				continue
			}

			lm.Lock()
			l = nl
			lm.Unlock()
		}
	}()

	for {
		s, err := probe()
		if err != nil {
			println(err.Error())
			s = &probing.Statistics{
				PacketLoss: 100,
			}
		}

		lm.Lock()
		wr := statisticsToWriteRequest(s, l)
		lm.Unlock()
		wrb <- wr

		time.Sleep(time.Second * 10)
	}
}

func getLabels() ([]prompb.Label, error) {
	gwIP, err := gateway.DiscoverGatewayIPv4()
	if err != nil {
		return nil, err
	}

	resp, err := http.Get("http://ip-api.com/json/?fields=66846719")
	if err != nil {
		return nil, err
	}

	hn, err := os.Hostname()
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		respOut, _ := httputil.DumpResponse(resp, true)
		return nil, errors.New(string(respOut))
	}

	type RM struct {
		Status        string  `json:"status"`
		Continent     string  `json:"continent"`
		ContinentCode string  `json:"continentCode"`
		Country       string  `json:"country"`
		CountryCode   string  `json:"countryCode"`
		Region        string  `json:"region"`
		RegionName    string  `json:"regionName"`
		City          string  `json:"city"`
		District      string  `json:"district"`
		Zip           string  `json:"zip"`
		Lat           float64 `json:"lat"`
		Lon           float64 `json:"lon"`
		Timezone      string  `json:"timezone"`
		Offset        int     `json:"offset"`
		Currency      string  `json:"currency"`
		Isp           string  `json:"isp"`
		Org           string  `json:"org"`
		As            string  `json:"as"`
		Asname        string  `json:"asname"`
		Reverse       string  `json:"reverse"`
		Mobile        bool    `json:"mobile"`
		Proxy         bool    `json:"proxy"`
		Hosting       bool    `json:"hosting"`
		Query         string  `json:"query"`
	}

	bb, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var r RM
	err = json.Unmarshal(bb, &r)
	if err != nil {
		return nil, err
	}

	return []prompb.Label{
		{
			Name:  "client_id",
			Value: os.Getenv("PINGPG_CLIENTID"),
		},
		{
			Name:  "hostname",
			Value: hn,
		},
		{
			Name:  "public_ip",
			Value: r.Query,
		},
		{
			Name:  "gateway",
			Value: gwIP.String(),
		},
		{
			Name:  "isp",
			Value: r.Isp,
		},
		{
			Name:  "country",
			Value: r.Country,
		},
		{
			Name:  "region",
			Value: r.RegionName,
		},
		{
			Name:  "city",
			Value: r.City,
		},
		{
			Name:  "mobile",
			Value: strconv.FormatBool(r.Mobile),
		},
		{
			Name:  "proxy",
			Value: strconv.FormatBool(r.Proxy),
		},
		{
			Name:  "hosting",
			Value: strconv.FormatBool(r.Hosting),
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

	if os.Getenv("PINGPG_DEBUG") == "true" {
		reqOut, _ := httputil.DumpRequestOut(req, true)
		println(string(reqOut))
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		respOut, _ := httputil.DumpResponse(resp, true)
		return errors.New(string(respOut))
	}

	if os.Getenv("PINGPG_DEBUG") == "true" {
		respOut, _ := httputil.DumpResponse(resp, true)
		println(string(respOut))
	}

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
	pinger.Timeout = time.Second * 3

	err = pinger.Run()
	if err != nil {
		return nil, err
	}

	return pinger.Statistics(), nil
}
