/*
Copyright 2024 Netic A/S.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
