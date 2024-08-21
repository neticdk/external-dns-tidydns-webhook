package main

import (
	"log/slog"
	"strings"
	"time"

	"github.com/neticdk/external-dns-tidydns-webhook/cmd/webhook/tidydns"
)

type ZoneProvider interface {
	getZones() []tidydns.Zone
}

type zoneProvider chan chan []tidydns.Zone

// For most requests a list of zones is needed, so to not make that many call to
// Tidy and delay the request processing this zone provider acts as a cache for
// the zone list. It's operated upon with messageing and initilly block any
// calls until the list of zones has been populated. After initialization the
// zone list is re-fetched every 10 minutes.
func newZoneProvider(tidy tidydns.TidyDNSClient, updateInterval time.Duration) ZoneProvider {
	provider := make(zoneProvider, 1)
	go func() {
		// Get all tidy zones
		zones, err := tidy.ListZones()
		if err != nil {
			panic(err.Error())
		}

		zonesStr := []string{}
		for _, v := range zones {
			zonesStr = append(zonesStr, v.Name)
		}

		slog.Debug("DNS zones: " + strings.Join(zonesStr, ", "))

		ticker := time.NewTicker(updateInterval)

		for {
			select {
			case respChan := <-provider:
				respChan <- zones
			case <-ticker.C:
				zones, err := tidy.ListZones()
				if err != nil {
					continue
				}

				zonesStr = []string{}
				for _, v := range zones {
					zonesStr = append(zonesStr, v.Name)
				}
			}
		}
	}()

	return provider
}

func (provider zoneProvider) getZones() []tidydns.Zone {
	responder := make(chan []tidydns.Zone, 1)
	provider <- responder
	return <-responder
}
