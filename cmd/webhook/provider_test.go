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
	"testing"
	"time"

	"github.com/neticdk/external-dns-tidydns-webhook/cmd/webhook/tidydns"
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
)

type mockTidyDNSClient struct {
	zones            []tidydns.Zone
	createdRecords   []tidydns.Record
	deletedRecordIds []json.Number
	err              error
}

func (m *mockTidyDNSClient) CreateRecord(zoneID json.Number, record *tidydns.Record) error {
	if m.err != nil {
		return m.err
	}

	m.createdRecords = append(m.createdRecords, *record)
	return nil
}

func (m *mockTidyDNSClient) ListRecords(zoneID json.Number) ([]tidydns.Record, error) {
	if m.err != nil {
		return nil, m.err
	}

	return m.createdRecords, nil
}

func (m *mockTidyDNSClient) DeleteRecord(zoneID json.Number, recordID json.Number) error {
	if m.err != nil {
		return m.err
	}

	m.deletedRecordIds = append(m.deletedRecordIds, recordID)
	return nil
}

func (m *mockTidyDNSClient) ListZones() ([]tidydns.Zone, error) {
	return m.zones, m.err
}

type mockZoneProvider struct{}

func (m *mockZoneProvider) getZones() []tidydns.Zone {
	return []tidydns.Zone{
		{Name: "example.com"},
	}
}

func TestNewProvider(t *testing.T) {
	tidy := &mockTidyDNSClient{}
	zoneUpdateInterval := 10 * time.Minute
	provider := newProvider(tidy, zoneUpdateInterval)

	if provider.tidy != tidy {
		t.Errorf("expected tidy to be %v, got %v", tidy, provider.tidy)
	}

	if provider.zoneProvider == nil {
		t.Error("expected zoneProvider to be initialized")
	}
}

func TestGetDomainFilter(t *testing.T) {
	tidy := &mockTidyDNSClient{}
	zoneProvider := &mockZoneProvider{}
	provider := &tidyProvider{
		tidy:         tidy,
		zoneProvider: zoneProvider,
	}

	domainFilter := provider.GetDomainFilter()
	expectedDomains := []string{"example.com"}

	for _, domain := range expectedDomains {
		if !domainFilter.Match(domain) {
			t.Errorf("expected domain filter to match %s", domain)
		}
	}
}

func TestRecords(t *testing.T) {
	tidy := &mockTidyDNSClient{}
	zoneProvider := &mockZoneProvider{}
	provider := &tidyProvider{
		tidy:         tidy,
		zoneProvider: zoneProvider,
	}

	tests := []struct {
		name           string
		mockRecords    []tidydns.Record
		expectedError  bool
		expectedResult []*Endpoint
	}{
		{
			name: "Valid A record",
			mockRecords: []tidydns.Record{
				{
					ID:          "1",
					Type:        "A",
					Name:        "test",
					Destination: "1.2.3.4",
					TTL:         json.Number("300"),
					ZoneName:    "example.com",
					ZoneID:      "1",
				},
			},
			expectedError: false,
			expectedResult: []*Endpoint{
				endpoint.NewEndpointWithTTL("test.example.com", "A", 300, "1.2.3.4"),
			},
		},
		{
			name: "Fail to list records",
			mockRecords: []tidydns.Record{
				{
					ID:          "2",
					Type:        "A",
					Name:        "test",
					Destination: "1.2.3.4",
					TTL:         json.Number("300"),
					ZoneName:    "example.com",
					ZoneID:      "1",
				},
			},
			expectedError:  true,
			expectedResult: nil,
		},
		{
			name: "Invalid TTL",
			mockRecords: []tidydns.Record{
				{
					ID:          "2",
					Type:        "A",
					Name:        "invalid-ttl",
					Destination: "1.2.3.4",
					TTL:         json.Number("300.2"),
					ZoneName:    "example.com",
					ZoneID:      "1",
				},
			},
			expectedError:  false,
			expectedResult: []*Endpoint{},
		},
		{
			name: "Multiple records",
			mockRecords: []tidydns.Record{
				{
					ID:          "3",
					Type:        "A",
					Name:        "multi",
					Destination: "1.2.3.4",
					TTL:         json.Number("300"),
					ZoneName:    "example.com",
					ZoneID:      "1",
				},
				{
					ID:          "4",
					Type:        "A",
					Name:        "multi",
					Destination: "5.6.7.8",
					TTL:         json.Number("300"),
					ZoneName:    "example.com",
					ZoneID:      "1",
				},
			},
			expectedError: false,
			expectedResult: []*Endpoint{
				endpoint.NewEndpointWithTTL("multi.example.com", "A", 300, "1.2.3.4", "5.6.7.8"),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tidy.createdRecords = test.mockRecords
			if test.expectedError {
				tidy.err = fmt.Errorf("list records error")
			} else {
				tidy.err = nil
			}

			records, err := provider.Records(context.Background())

			if test.expectedError {
				if err == nil {
					t.Fatalf("expected error, got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			if len(records) != len(test.expectedResult) {
				fmt.Println(records)
				t.Fatalf("expected %d records, got %d", len(test.expectedResult), len(records))
			}

			for i, record := range records {
				if record.DNSName != test.expectedResult[i].DNSName || record.RecordType != test.expectedResult[i].RecordType || record.RecordTTL != test.expectedResult[i].RecordTTL || len(record.Targets) != len(test.expectedResult[i].Targets) || record.Targets[0] != test.expectedResult[i].Targets[0] {
					t.Errorf("expected %v, got %v", test.expectedResult[i], record)
				}
			}
		})
	}
}

func TestAdjustEndpoints(t *testing.T) {
	// Labels are not added by the constructor, so we add them manually after
	// the fact and use them as test parameters below.
	ARecWithLabels := endpoint.NewEndpointWithTTL("example.com", "A", 100, "1.2.3.4")
	ARecWithLabels.Labels = map[string]string{"label": "value", "label2": "value2"}

	TXTRecWithLabels := endpoint.NewEndpointWithTTL("example.com", "TXT", 300, "\"v=spf1 include:example.com ~all\"")
	TXTRecWithLabels.Labels = map[string]string{"label": "value", "label2": "value2"}

	tests := []struct {
		name      string
		endpoints []*Endpoint
		expected  []*Endpoint
	}{
		{
			name:      "Adjust TTL and remove labels",
			endpoints: []*Endpoint{ARecWithLabels, TXTRecWithLabels},
			expected: []*Endpoint{
				endpoint.NewEndpointWithTTL("example.com", "A", 300, "1.2.3.4"),
				endpoint.NewEndpointWithTTL("example.com", "TXT", 300, "\"v=spf1 include:example.com ~all\""),
			},
		},
		{
			name: "Adjust TTL to minimum and encode punycode",
			endpoints: []*Endpoint{
				endpoint.NewEndpointWithTTL("xn--exmple-cua.com", "A", 100, "1.2.3.4"),
			},
			expected: []*Endpoint{
				endpoint.NewEndpointWithTTL("xn--exmple-cua.com", "A", 300, "1.2.3.4"),
			},
		},
		{
			name: "No adjustment needed",
			endpoints: []*Endpoint{
				endpoint.NewEndpointWithTTL("example.com", "A", 300, "1.2.3.4"),
			},
			expected: []*Endpoint{
				endpoint.NewEndpointWithTTL("example.com", "A", 300, "1.2.3.4"),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tidy := &mockTidyDNSClient{}
			provider := &tidyProvider{
				tidy:         tidy,
				zoneProvider: &mockZoneProvider{},
			}

			result, err := provider.AdjustEndpoints(test.endpoints)
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			if len(result) != len(test.expected) {
				t.Fatalf("expected %d endpoints, got %d", len(test.expected), len(result))
			}

			for i, endpoint := range result {
				if endpoint.DNSName != test.expected[i].DNSName || endpoint.RecordType != test.expected[i].RecordType || endpoint.RecordTTL != test.expected[i].RecordTTL || len(endpoint.Targets) != len(test.expected[i].Targets) || endpoint.Targets[0] != test.expected[i].Targets[0] {
					t.Errorf("expected %v, got %v", test.expected[i], endpoint)
				}
			}
		})
	}
}

func TestApplyChanges(t *testing.T) {
	tidy := &mockTidyDNSClient{}
	zoneProvider := &mockZoneProvider{}
	provider := &tidyProvider{
		tidy:         tidy,
		zoneProvider: zoneProvider,
	}

	tests := []struct {
		name      string
		changes   *plan.Changes
		expectErr bool
	}{
		{
			name:      "Create record",
			expectErr: false,
			changes: &plan.Changes{
				Create: []*Endpoint{
					endpoint.NewEndpointWithTTL("create.example.com", "A", 300, "1.2.3.4"),
				},
			},
		},
		{
			name:      "Delete record",
			expectErr: false,
			changes: &plan.Changes{
				Delete: []*Endpoint{
					endpoint.NewEndpointWithTTL("delete.example.com", "A", 300, "1.2.3.4"),
				},
			},
		},
		{
			name:      "Update record",
			expectErr: false,
			changes: &plan.Changes{
				UpdateOld: []*Endpoint{
					endpoint.NewEndpointWithTTL("update.example.com", "A", 300, "1.2.3.4"),
				},
				UpdateNew: []*Endpoint{
					endpoint.NewEndpointWithTTL("update.example.com", "A", 300, "5.6.7.8"),
				},
			},
		},
		{
			name:      "Fail updating record",
			expectErr: true,
			changes: &plan.Changes{
				UpdateOld: []*Endpoint{
					endpoint.NewEndpointWithTTL("update.example.com", "A", 300, "1.2.3.4"),
				},
				UpdateNew: []*Endpoint{
					endpoint.NewEndpointWithTTL("update.example.com", "A", 300, "5.6.7.8"),
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.expectErr {
				tidy.err = fmt.Errorf("apply changes error")
			} else {
				tidy.err = nil
			}

			err := provider.ApplyChanges(context.Background(), test.changes)
			if !test.expectErr && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}

func TestDeleteEndpoint(t *testing.T) {
	allRecords := []tidydns.Record{
		{
			ID:          "1",
			Type:        "A",
			Name:        "delete",
			Destination: "1.2.3.4",
			TTL:         json.Number("300"),
			ZoneName:    "example.com",
			ZoneID:      "1",
		},
		{
			ID:          "2",
			Type:        "CNAME",
			Name:        "www",
			Destination: "example.com",
			TTL:         json.Number("300"),
			ZoneName:    "example.com",
			ZoneID:      "1",
		},
	}

	tests := []struct {
		name         string
		encounterErr error
		endpoint     *Endpoint
		expected     []json.Number
	}{
		{
			name:         "Delete A record",
			encounterErr: nil,
			endpoint:     endpoint.NewEndpointWithTTL("delete.example.com", "A", 300, "1.2.3.4"),
			expected: []json.Number{
				json.Number("1"),
			},
		},
		{
			name:         "Delete CNAME record",
			encounterErr: nil,
			endpoint:     endpoint.NewEndpointWithTTL("www.example.com", "CNAME", 300, "example.com"),
			expected: []json.Number{
				json.Number("2"),
			},
		},
		{
			name:         "Delete non-existing record",
			encounterErr: nil,
			endpoint:     endpoint.NewEndpointWithTTL("nonexistent.example.com", "A", 300, "1.2.3.4"),
			expected:     []json.Number{},
		},
		{
			name:         "Error on delete",
			encounterErr: fmt.Errorf("delete record error"),
			endpoint:     endpoint.NewEndpointWithTTL("delete.example.com", "A", 300, "1.2.3.4"),
			expected:     []json.Number{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tidy := &mockTidyDNSClient{
				err: test.encounterErr,
			}

			provider := &tidyProvider{
				tidy:         tidy,
				zoneProvider: &mockZoneProvider{},
			}

			provider.deleteEndpoint(allRecords, test.endpoint)

			if len(tidy.deletedRecordIds) != len(test.expected) {
				t.Fatalf("expected %d records to be deleted, got %d", len(test.expected), len(tidy.deletedRecordIds))
			}

			for i, recordId := range tidy.deletedRecordIds {
				if recordId != test.expected[i] {
					t.Errorf("expected record ID %s, got %s", test.expected[i], tidy.deletedRecordIds[i])
				}
			}
		})
	}
}

func TestCreateRecord(t *testing.T) {
	zones := []tidydns.Zone{
		{Name: "example.com", ID: "1"},
		{Name: "example.org", ID: "2"},
	}

	tests := []struct {
		name         string
		zones        []tidydns.Zone
		encounterErr error
		endpoint     *Endpoint
		expected     []tidydns.Record
	}{
		{
			name:         "Create A record",
			zones:        zones,
			encounterErr: nil,
			endpoint:     endpoint.NewEndpointWithTTL("create.example.com", "A", 300, "1.2.3.4"),
			expected: []tidydns.Record{
				{
					Type:        "A",
					Name:        "create",
					Destination: "1.2.3.4",
					TTL:         json.Number("300"),
				},
			},
		},
		{
			name:         "Error on create A record",
			zones:        zones,
			encounterErr: fmt.Errorf("create record error"),
			endpoint:     endpoint.NewEndpointWithTTL("create.example.com", "A", 300, "1.2.3.4"),
			expected:     []tidydns.Record{},
		},
		{
			name:         "Create CNAME record",
			zones:        zones,
			encounterErr: nil,
			endpoint:     endpoint.NewEndpointWithTTL("www.example.com", "CNAME", 300, "example.com"),
			expected: []tidydns.Record{
				{
					Type:        "CNAME",
					Name:        "www",
					Destination: "example.com.",
					TTL:         json.Number("300"),
				},
			},
		},
		{
			name:         "Create TXT record",
			zones:        zones,
			encounterErr: nil,
			endpoint:     endpoint.NewEndpointWithTTL("txt.example.com", "TXT", 300, "\"v=spf1 include:example.com ~all\""),
			expected: []tidydns.Record{
				{
					Type:        "TXT",
					Name:        "txt",
					Destination: "v=spf1 include:example.com ~all",
					TTL:         json.Number("300"),
				},
			},
		},
		{
			name:         "Create record with TTL below minimum",
			zones:        zones,
			encounterErr: nil,
			endpoint:     endpoint.NewEndpointWithTTL("lowttl.example.com", "A", 100, "1.2.3.4"),
			expected: []tidydns.Record{
				{
					Type:        "A",
					Name:        "lowttl",
					Destination: "1.2.3.4",
					TTL:         json.Number("300"),
				},
			},
		},
		{
			name:         "Create record with no zones",
			zones:        []tidydns.Zone{},
			encounterErr: nil,
			endpoint:     endpoint.NewEndpointWithTTL("nozone.example.com", "A", 300, "1.2.3.4"),
			expected:     []tidydns.Record{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tidy := &mockTidyDNSClient{
				err: test.encounterErr,
			}

			provider := &tidyProvider{
				tidy:         tidy,
				zoneProvider: &mockZoneProvider{},
			}

			provider.createRecord(test.zones, test.endpoint)

			if len(tidy.createdRecords) != len(test.expected) {
				t.Fatalf("expected %d records to be created, got %d", len(test.expected), len(tidy.createdRecords))
			}

			for i, record := range tidy.createdRecords {
				if record.Type != test.expected[i].Type || record.Name != test.expected[i].Name || record.Destination != test.expected[i].Destination || record.TTL != test.expected[i].TTL {
					t.Errorf("expected record %+v, got %+v", test.expected[i], record)
				}
			}
		})
	}
}

func TestParseTidyRecord(t *testing.T) {
	tests := []struct {
		name     string
		record   tidyRecord
		expected *Endpoint
	}{
		{
			name: "A record",
			record: tidyRecord{
				ID:          "1",
				Type:        "A",
				Name:        "example",
				Description: "Test A record",
				Destination: "1.2.3.4",
				TTL:         "300",
				ZoneName:    "example.com",
				ZoneID:      "1",
			},
			expected: endpoint.NewEndpointWithTTL("example.example.com", "A", 300, "1.2.3.4"),
		},
		{
			name: "CNAME record",
			record: tidyRecord{
				ID:          "2",
				Type:        "CNAME",
				Name:        "www",
				Description: "Test CNAME record",
				Destination: "example.com.",
				TTL:         "300",
				ZoneName:    "example.com",
				ZoneID:      "1",
			},
			expected: endpoint.NewEndpointWithTTL("www.example.com", "CNAME", 300, "example.com"),
		},
		{
			name: "TXT record",
			record: tidyRecord{
				ID:          "3",
				Type:        "TXT",
				Name:        "txt",
				Description: "Test TXT record",
				Destination: "\"v=spf1 include:example.com ~all\"",
				TTL:         "300",
				ZoneName:    "example.com",
				ZoneID:      "1",
			},
			expected: endpoint.NewEndpointWithTTL("txt.example.com", "TXT", 300, "\"v=spf1 include:example.com ~all\""),
		},
		{
			name: "Invalid TTL",
			record: tidyRecord{
				ID:          "4",
				Type:        "A",
				Name:        "invalid-ttl",
				Description: "Test invalid TTL",
				Destination: "1.2.3.4",
				TTL:         "invalid",
				ZoneName:    "example.com",
				ZoneID:      "1",
			},
			expected: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := parseTidyRecord(&test.record)
			if result == nil && test.expected != nil {
				t.Errorf("expected %v, got nil", test.expected)
			} else if result != nil && test.expected == nil {
				t.Errorf("expected nil, got %v", result)
			} else if result != nil && test.expected != nil {
				if result.DNSName != test.expected.DNSName || result.RecordType != test.expected.RecordType || result.RecordTTL != test.expected.RecordTTL || len(result.Targets) != len(test.expected.Targets) || result.Targets[0] != test.expected.Targets[0] {
					t.Errorf("expected %v, got %v", test.expected, result)
				}
			}
		})
	}
}

func TestTidyNameToFQDN(t *testing.T) {
	tests := []struct {
		name      string
		inputName string
		inputZone string
		expected  string
	}{
		{"Root domain", ".", "example.com", "example.com"},
		{"Subdomain", "sub", "example.com", "sub.example.com"},
		{"Root domain with dot", ".", "example.org", "example.org"},
		{"Subdomain with dot", "sub", "example.org", "sub.example.org"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := tidyNameToFQDN(test.inputName, test.inputZone)
			if result != test.expected {
				t.Errorf("expected %s, got %s", test.expected, result)
			}
		})
	}
}

func TestClampTTL(t *testing.T) {
	tests := []struct {
		name     string
		inputTTL int
		expected int
	}{
		{"TTL below minimum", 100, 300},
		{"TTL at minimum", 300, 300},
		{"TTL above minimum", 600, 600},
		{"TTL zero", 0, 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := clampTTL(test.inputTTL)
			if result != test.expected {
				t.Errorf("expected %d, got %d", test.expected, result)
			}
		})
	}
}

func TestTidyfyName(t *testing.T) {
	zones := []tidydns.Zone{
		{Name: "example.com", ID: "1"},
		{Name: "example.org", ID: "2"},
	}

	tests := []struct {
		name     string
		fqdn     string
		expected string
		zoneID   json.Number
	}{
		{"Root domain", "example.com", ".", "1"},
		{"Subdomain", "sub.example.com", "sub", "1"},
		{"Root domain org", "example.org", ".", "2"},
		{"Subdomain org", "sub.example.org", "sub", "2"},
		{"Non-matching domain", "example.net", "", "0"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, zoneID := tidyfyName(zones, test.fqdn)
			if result != test.expected || zoneID != test.zoneID {
				t.Errorf("expected (%s, %s), got (%s, %s)", test.expected, test.zoneID, result, zoneID)
			}
		})
	}
}
