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
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"

	"github.com/neticdk/external-dns-tidydns-webhook/cmd/webhook/tidydns"
	"golang.org/x/net/idna"
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
	"sigs.k8s.io/external-dns/provider"
)

type tidyProvider struct {
	tidy         tidydns.TidyDNSClient
	zoneProvider ZoneProvider
}

type Provider = provider.Provider
type Endpoint = endpoint.Endpoint
type tidyRecord = tidydns.Record

func newProvider(tidy tidydns.TidyDNSClient, zoneProvider ZoneProvider) *tidyProvider {
	return &tidyProvider{
		tidy:         tidy,
		zoneProvider: zoneProvider,
	}
}

// Get list of zones from Tidy and return a domain filter based on them.
func (p *tidyProvider) GetDomainFilter() endpoint.DomainFilterInterface {
	// Make list of all zone names
	zoneNames := []string{}
	for _, zone := range p.zoneProvider.getZones() {
		zoneNames = append(zoneNames, zone.Name)
	}

	// Make domain filter
	return endpoint.NewDomainFilter(zoneNames)
}

// Return a list of all DNS records in Tidy. An endpoint in External-DNS can
// have multiple targets (called distination in Tidy). Tidy does not support
// this so multiple records are instead created when this is necessary. This
// function attempts to merge these together when reporting back to
// External-DNS.
func (p *tidyProvider) Records(ctx context.Context) ([]*Endpoint, error) {
	allRecords, err := p.allRecords()
	if err != nil {
		return nil, err
	}

	endpoints := []*Endpoint{}

	for _, record := range allRecords {
		endpoint := parseTidyRecord(&record)
		if endpoint == nil {
			continue
		}

		index := -1
		for i := range endpoints {
			if endpoints[i].DNSName == endpoint.DNSName && endpoints[i].RecordType == endpoint.RecordType {
				index = i
			}
		}

		if index != -1 {
			targets := &endpoints[index].Targets
			*targets = append(*targets, endpoint.Targets...)
		} else {
			endpoints = append(endpoints, endpoint)
		}
	}

	return endpoints, nil
}

// Adjust endpoints to how they would look, when comming from Tidy. Some
// exceptions are made like not using the FQDN and a multi-target endpoint being
// multiple records. Theese changes are invisible to External-DNS. However
// things like the TTL restrictions, labels not being supported and unicode
// being punycode encoded is applied in this function.
func (p *tidyProvider) AdjustEndpoints(endpoints []*Endpoint) ([]*Endpoint, error) {
	for _, v := range endpoints {
		// Restrict TTL to permitted range by Tidy DNS
		v.RecordTTL = endpoint.TTL(restrictTTL(int(v.RecordTTL)))

		// Labels are not supported hence removed
		v.Labels = endpoint.Labels{}

		// Any unicode is encoded as punycode
		v.DNSName, _ = idna.Lookup.ToASCII(v.DNSName)
	}

	return endpoints, nil
}

// Create, delete or change records. We use a list of zones since External-DNS
// doesn't know and we need the zone name to adjust DNS name and zoneID to apply
// changes in Tidy. It's assumed that update_old and update_new has equal number
// of entries. Instead of changing records in-place, old records and simly
// deleted and their corrections are created as new records.
func (p *tidyProvider) ApplyChanges(ctx context.Context, changes *plan.Changes) error {
	zones := p.zoneProvider.getZones()
	wg := sync.WaitGroup{}

	for _, create := range changes.Create {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.createRecord(zones, create)
		}()
	}

	allRecords, err := p.allRecords()
	if err != nil {
		return err
	}

	for _, delete := range changes.Delete {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.deleteEndpoint(allRecords, delete)
		}()
	}

	for _, old := range changes.UpdateOld {
		p.deleteEndpoint(allRecords, old)
	}

	for _, new := range changes.UpdateNew {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.createRecord(zones, new)
		}()
	}

	wg.Wait()

	return nil
}

// Convert a Tidy record into an External-DNS endpoint. This potentially changes
// the TTL, the content of a TXT record and the DNS name.
func parseTidyRecord(record *tidyRecord) *Endpoint {
	// Convert DNS name into a FQDN
	var dnsName string
	if record.Name == "." {
		dnsName = record.ZoneName
	} else {
		dnsName = record.Name + "." + record.ZoneName
	}

	if dnsName == "" {
		return nil
	}

	ttlTemp, err := record.TTL.Int64()
	if err != nil {
		slog.Warn(err.Error())
		return nil
	}

	// Convert TTL to TTL type
	ttl := endpoint.TTL(ttlTemp)

	if record.Type == "CNAME" {
		record.Destination = strings.TrimRight(record.Destination, ".")
	}

	// Create Endpoint
	return endpoint.NewEndpointWithTTL(dnsName, record.Type, ttl, record.Destination)
}

// Fetch and create a list of all records from all zones
func (p *tidyProvider) allRecords() ([]tidyRecord, error) {
	zones := p.zoneProvider.getZones()

	allRecords := []tidyRecord{}

	for _, zone := range zones {
		records, err := p.tidy.ListRecords(zone.ID)
		if err != nil {
			return nil, err
		}

		allRecords = append(allRecords, records...)
	}

	return allRecords, nil
}

func (p *tidyProvider) deleteEndpoint(allRecords []tidyRecord, endpoint *Endpoint) {
	foundRecords := findRecords(allRecords, endpoint)
	if len(foundRecords) == 0 {
		return
	}

	for _, record := range foundRecords {
		slog.Debug(fmt.Sprintf("delete record %+v", record))
		if err := p.tidy.DeleteRecord(record.ZoneID, record.ID); err != nil {
			return
		}
	}
}

// Find all matching records from a list. Since one endpoint cam have multiple
// targets they can represent multiple records in Tidy.
func findRecords(records []tidyRecord, endpoint *Endpoint) []tidyRecord {
	found := []tidydns.Record{}
	for _, target := range endpoint.Targets {
		for _, record := range records {
			dnsName := ""
			if record.Name == "." {
				dnsName = record.ZoneName
			} else {
				dnsName = record.Name + "." + record.ZoneName
			}

			if dnsName == endpoint.DNSName && record.Type == endpoint.RecordType && record.Destination == target {
				found = append(found, record)
			}
		}
	}

	return found
}

// Create record(s) from an External-DNS endpoint. As endpoints can have
// potentially multiple targets, we may create multiple records which is also
// handled here.
func (p *tidyProvider) createRecord(zones []tidydns.Zone, endpoint *Endpoint) {
	dnsName, zoneID := tidyfyName(zones, endpoint.DNSName)
	if dnsName == "" {
		slog.Debug(fmt.Sprintf("DNS name %s cannot be mapped", endpoint.DNSName))
		return
	}

	ttl := restrictTTL(int(endpoint.RecordTTL))

	for _, target := range endpoint.Targets {
		// For some reason external-dns wraps the value of certain TXT records
		// with extra double quotes. This isn't supported by Tidy and it will
		// refuse to save and removing them seemingly causes no issues for
		// external-dns when read back.
		target = strings.Trim(target, "\"")

		if endpoint.RecordType == "CNAME" {
			target += "."
		}

		newRec := &tidyRecord{
			Type:        endpoint.RecordType,
			Name:        dnsName,
			Description: "",
			Destination: target,
			TTL:         json.Number(strconv.Itoa(ttl)),
		}

		slog.Debug(fmt.Sprintf("create record %+v", *newRec))
		if err := p.tidy.CreateRecord(zoneID, newRec); err != nil {
			slog.Warn(err.Error())
			slog.Debug(fmt.Sprintf("%+v", *newRec))
			return
		}
	}
}

// Handles sanitizing TTL to Tidy. TidyDNS doesn't support TTL under 300 except
// 0 which is the namespace default value
func restrictTTL(ttl int) int {
	if ttl != 0 && ttl < 300 {
		return 300
	}

	return ttl
}

// Convert FQDNs into Tidy DNS names. External-DNS communicates DNS names using
// the FQDN where-as Tidy strips away the namespace and uses '.' when the
// namespace is the FQDN.
func tidyfyName(zones []tidydns.Zone, name string) (string, json.Number) {
	for _, zone := range zones {
		if !strings.HasSuffix(name, zone.Name) {
			continue
		}

		if cutted, _ := strings.CutSuffix(name, zone.Name); cutted != "" {
			cutted, _ = strings.CutSuffix(cutted, ".")
			return cutted, zone.ID
		}

		return ".", zone.ID
	}

	return "", "0"
}
