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

	// Get all tidy zones
	zones, err := tidy.ListZones()
	if err != nil {
		panic(err.Error())
	}

	ticker := time.NewTicker(updateInterval)

	go func() {
		for {
			select {
			case respChan := <-provider:
				respChan <- zones
			case <-ticker.C:
				if zones, err = tidy.ListZones(); err != nil {
					slog.Error("error updating zones", "error", err)
					continue
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
