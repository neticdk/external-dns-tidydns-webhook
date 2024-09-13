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
	"flag"
	"fmt"
	"log/slog"
	"os"
	"runtime/debug"
	"time"

	"github.com/neticdk/external-dns-tidydns-webhook/cmd/webhook/tidydns"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/sdk/metric"
)

func main() {
	tidyEndpoint := flag.String("tidydns-endpoint", "", "DNS server address")
	logLevel := flag.String("log-level", "", "logging level (debug, info, warn, err)")

	zoneArgDescription := "The intercval at which to update zone information format 00h00m00s e.g. 1h32m"
	zoneUpdateIntervalArg := flag.String("zone-update-interval", "10m", zoneArgDescription)

	flag.Parse()

	tidyUsername := os.Getenv("TIDYDNS_USER")
	tidyPassword := os.Getenv("TIDYDNS_PASS")

	// If log level isn't set as a parameter, read it from the environment
	if *logLevel == "" {
		*logLevel = os.Getenv("EXTERNAL_DNS_LOG_LEVEL")
	}

	// If neither the application parameter or the environment sets the log
	// level, set default
	if *logLevel == "" {
		*logLevel = "info"
	}

	loggingSetup(*logLevel, os.Stderr, true)

	// Print stachtraces with slog
	defer func() {
		if err := recover(); err != nil {
			stackTrace := string(debug.Stack())
			msg := fmt.Sprintf("panic: %v\n\n%s", err, stackTrace)
			slog.Error(msg)
		}
	}()

	// Parse the interval deciding how often the zone information is updated
	zoneUpdateInterval, err := time.ParseDuration(*zoneUpdateIntervalArg)
	if err != nil {
		panic(err.Error())
	}

	// Create a Prometheus reader/exporter
	prom, err := prometheus.New(prometheus.WithoutScopeInfo())
	if err != nil {
		panic(err.Error())
	}

	// Use the exporter to make a meter for Tidy to attach instrumentation
	meterProvider := metric.NewMeterProvider(metric.WithReader(prom))
	tidyMeter := meterProvider.Meter("tidy")

	// Make a Tidy object to abstract calls to Tidy
	tidy, err := tidydns.NewTidyDnsClient(*tidyEndpoint, tidyUsername, tidyPassword, (10 * time.Second), tidyMeter)
	if err != nil {
		panic(err.Error())
	}

	// Make zoneprovider to fetch the zone information with at the set interval
	zoneProvider := newZoneProvider(tidy, zoneUpdateInterval)

	// With the Tidy object, make a provider to handle the logic and conversions
	// between External-DNS and Tidy
	provider, err := newProvider(tidy, zoneProvider)
	if err != nil {
		panic(err.Error())
	}

	// Use the provider to make a webhook containing all the callable endpoints
	webhook := newWebhook(provider)

	// Start webserver to service requests from External-DNS
	go func() {
		if err = serveWebhook(webhook, "127.0.0.1:8888"); err != nil {
			panic(err.Error())
		}
	}()

	metricsHandler := promhttp.Handler()

	// Start website to service metrics and health check
	if err = serveExposed("0.0.0.0:8080", metricsHandler); err != nil {
		panic(err.Error())
	}
}
