package main

import (
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

func serveExposed(addr string, metricsHandler http.Handler) error {
	slog.Debug("start webhook server on " + addr)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthz)
	mux.Handle("GET /metrics", metricsHandler)

	server := http.Server{
		Addr:    addr,
		Handler: mux,
	}

	return server.ListenAndServe()
}

func healthz(w http.ResponseWriter, req *http.Request) {
	w.WriteHeader(http.StatusOK)
}
