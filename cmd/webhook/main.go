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
	log "github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/sdk/metric"
	"sigs.k8s.io/external-dns/provider/webhook/api"
)

type config struct {
	logLevel           string
	logFormat          string
	tidyEndpoint       string
	readTimeout        time.Duration
	writeTimeout       time.Duration
	zoneUpdateInterval time.Duration
	tidyUsername       string
	tidyPassword       string
}

func main() {
	cfg, parsingErr := parseConfig()

	// Setup the default slog logger
	loggingSetup(cfg.logFormat, cfg.logLevel, os.Stderr, true)

	// External DNS uses logrus for logging, so we set that up as well
	if cfg.logFormat == "json" {
		log.SetFormatter(&log.JSONFormatter{})
	} else {
		log.SetFormatter(&log.TextFormatter{})
	}

	// Print stachtraces with slog
	defer func() {
		if err := recover(); err != nil {
			stackTrace := string(debug.Stack())
			msg := fmt.Sprintf("panic: %v\n\n%s", err, stackTrace)
			slog.Error(msg)
		}
	}()

	// We check for parsingerrors here, as we want to log them with slog
	if parsingErr != nil {
		panic(parsingErr.Error())
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
	tidy, err := tidydns.NewTidyDnsClient(cfg.tidyEndpoint, cfg.tidyUsername, cfg.tidyPassword, (10 * time.Second), tidyMeter)
	if err != nil {
		panic(err.Error())
	}

	// With the Tidy object, make a provider to handle the logic and conversions
	// between External-DNS and Tidy
	provider := newProvider(tidy, cfg.zoneUpdateInterval)

	// Start webserver to service requests from External-DNS
	go api.StartHTTPApi(provider, nil, cfg.readTimeout, cfg.writeTimeout, "127.0.0.1:8888")

	metricsHandler := promhttp.Handler()

	// Start website to service metrics and health check
	if err = serveExposed("0.0.0.0:8080", metricsHandler); err != nil {
		panic(err.Error())
	}
}

func parseConfig() (*config, error) {
	logLevel := flag.String("log-level", "info", "Set the level of logging. (default: info, options: debug, info, warning, error)")
	logFormat := flag.String("log-format", "text", "The format in which log messages are printed (default: text, options: text, json)")
	tidyEndpoint := flag.String("tidydns-endpoint", "", "DNS server address")
	readTimeout := flag.Duration("read-timeout", (5 * time.Second), "Read timeout in duration format (default: 5s)")
	writeTimeout := flag.Duration("write-timeout", (10 * time.Second), "Write timeout in duration format (default: 10s)")

	zoneArgDescription := "The intercval at which to update zone information format 00h00m00s e.g. 1h32m"
	zoneUpdateIntervalArg := flag.String("zone-update-interval", "10m", zoneArgDescription)

	flag.Parse()

	tidyUsername := os.Getenv("TIDYDNS_USER")
	tidyPassword := os.Getenv("TIDYDNS_PASS")

	// Parse the interval deciding how often the zone information is updated
	zoneUpdateInterval, err := time.ParseDuration(*zoneUpdateIntervalArg)
	if err != nil {
		return nil, err
	}

	return &config{
		logLevel:           *logLevel,
		logFormat:          *logFormat,
		tidyEndpoint:       *tidyEndpoint,
		readTimeout:        *readTimeout,
		writeTimeout:       *writeTimeout,
		zoneUpdateInterval: zoneUpdateInterval,
		tidyUsername:       tidyUsername,
		tidyPassword:       tidyPassword,
	}, nil
}
