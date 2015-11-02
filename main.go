// A simple package to export basic information from the nats monitoring
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	addr        = flag.String("web.listen-address", ":9104", "Address on which to expose metrics and web interface.")
	natsaddr    = flag.String("nats.address", ":8043", "Address on which gnatsd is giving metrics")
	consumeTime = flag.Int("consume-time", 10, "Time between data scrapes from gnatsd monitoring")

	bytesCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "natsio",
		Name:      "bytes_total",
		Help:      "Total bytes in and out",
	},
		[]string{"direction"},
	)

	messageCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "natsio",
		Name:      "msg_total",
		Help:      "Total messages in and out",
	},
		[]string{"direction"},
	)

	connections = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "natsio",
		Name:      "connections",
		Help:      "Current active connections to gnatsd deamon",
	})
	slowConsumers = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "natsio",
		Name:      "slow_consumers",
		Help:      "Amount of slow consumers",
	})
)

func main() {
	flag.Parse()
	if *consumeTime < 1 {
		log.Fatal("consume time should be >0")
	}

	resp, err := http.Get(fmt.Sprintf("http://%s/varz", *natsaddr))
	if err != nil {
		log.Fatal(err)
	}

	if resp.StatusCode != http.StatusOK {
		log.Fatalf("expected statuscode 200 got %v", resp.StatusCode)
	}

	resp.Body.Close()

	prometheus.MustRegister(bytesCounter)
	prometheus.MustRegister(messageCounter)
	prometheus.MustRegister(connections)
	prometheus.MustRegister(slowConsumers)

	go consume()

	http.Handle("/metrics", prometheus.Handler())
	if err := http.ListenAndServe(*addr, nil); err != nil {
		log.Fatal(err)
	}
}

func consume() {
	for {
		time.Sleep(time.Second * time.Duration(*consumeTime))
		client := http.Client{
			Timeout: time.Second * time.Duration(*consumeTime),
		}

		resp, err := client.Get(fmt.Sprintf("http://%s/varz", *natsaddr))
		if err != nil {
			log.Print(err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			log.Printf("expected statuscode 200 got %v", resp.StatusCode)
			continue
		}

		var natsinfo struct {
			BytesIn  float64 `json:"in_bytes"`
			BytesOut float64 `json:"out_bytes"`

			MessagesIn  float64 `json:"in_msgs"`
			MessagesOut float64 `json:"out_msgs"`

			SlowConsumers float64 `json:"slow_consumers"`

			Connections    float64 `json:"connections"`
			MaxConnections float64 `json:"max_connections"`
		}

		err = json.NewDecoder(resp.Body).Decode(&natsinfo)
		if err != nil {
			log.Printf("could not decode nats-info got %v", err)
			resp.Body.Close()
			continue
		}
		resp.Body.Close()

		bytesCounter.WithLabelValues("in").Add(natsinfo.BytesIn)
		bytesCounter.WithLabelValues("out").Add(natsinfo.BytesOut)

		messageCounter.WithLabelValues("in").Add(natsinfo.MessagesIn)
		messageCounter.WithLabelValues("out").Add(natsinfo.MessagesOut)

		connections.Set(natsinfo.Connections)
		slowConsumers.Set(natsinfo.SlowConsumers)
	}
}
