package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"runtime/debug"
	"time"

	"github.com/neticdk/external-dns-tidydns-webhook/cmd/webhook/tidydns"
)

func main() {
	tidyEndpoint := flag.String("tidydns-endpoint", "", "DNS server address")
	logLevel := flag.String("log-level", "debug", "logging level (debug, info, warn, err)")

	zoneArgDescription := "The intercval at which to update zone information format 00h00m00s e.g. 1h32m"
	zoneUpdateIntervalArg := flag.String("zone-update-interval", "10m", zoneArgDescription)

	flag.Parse()

	tidyUsername := os.Getenv("TIDYDNS_USER")
	tidyPassword := os.Getenv("TIDYDNS_PASS")

	loggingSetup(*logLevel, os.Stderr, true)

	// Print stachtraces with slog
	defer func() {
		if err := recover(); err != nil {
			stackTrace := string(debug.Stack())
			msg := fmt.Sprintf("panic: %v\n\n%s", err, stackTrace)
			slog.Error(msg)
		}
	}()

	zoneUpdateInterval, err := time.ParseDuration(*zoneUpdateIntervalArg)
	if err != nil {
		panic(err.Error())
	}

	// Make a Tidy object to abstract calls to Tidy
	tidy := tidydns.NewTidyDnsClient(*tidyEndpoint, tidyUsername, tidyPassword, (10 * time.Second))

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

	// Start website to service metrics and health check
	if err = serveExposed("0.0.0.0:8080"); err != nil {
		panic(err.Error())
	}
}
