package main

import (
	"bytes"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/metrics"
)

type Samples []metrics.Sample

func serveWebhook(wh webhook, addr string) error {
	slog.Debug("start webhook server on " + addr)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", wh.negociate)
	mux.HandleFunc("GET /records", wh.getRecords)
	mux.HandleFunc("POST /adjustendpoints", wh.adjustEndpoints)
	mux.HandleFunc("POST /records", wh.applyChanges)
	mux.HandleFunc("GET /healthz", healthz)

	server := http.Server{
		Addr:    addr,
		Handler: mux,
	}

	return server.ListenAndServe()
}

func serveExposed(addr string) error {
	descs := metrics.All()

	samples := Samples(make([]metrics.Sample, len(descs)))
	for i := range samples {
		samples[i].Name = descs[i].Name
	}

	slog.Debug("start webhook server on " + addr)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthz)
	mux.HandleFunc("GET /metrics", samples.endpoint)

	server := http.Server{
		Addr:    addr,
		Handler: mux,
	}

	return server.ListenAndServe()
}

func healthz(w http.ResponseWriter, req *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (samples *Samples) endpoint(w http.ResponseWriter, _ *http.Request) {
	metrics.Read(*samples)

	resp := bytes.NewBuffer([]byte{})

	for _, sample := range *samples {
		name, value := sample.Name, sample.Value

		switch value.Kind() {
		case metrics.KindUint64:
			msg := fmt.Sprintf("%s: %d\n", name, value.Uint64())
			if _, err := resp.WriteString(msg); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		case metrics.KindFloat64:
			msg := fmt.Sprintf("%s: %f\n", name, value.Float64())
			if _, err := resp.WriteString(msg); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		case metrics.KindFloat64Histogram:
			msg := fmt.Sprintf("%s: %v\n", name, value.Float64Histogram())
			if _, err := resp.WriteString(msg); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		case metrics.KindBad:
			// This should never happen because all metrics are supported
			// by construction.
			panic("bug in runtime/metrics package!")
		default:
			// This may happen as new metrics get added.
			//
			// The safest thing to do here is to simply log it somewhere
			// as something to look into, but ignore it for now.
			// In the worst case, you might temporarily miss out on a new metric.
			msg := fmt.Sprintf("%s: unexpected metric Kind: %v\n", name, value.Kind())
			slog.Error(msg)
		}
	}

	w.Write(resp.Bytes())
}
