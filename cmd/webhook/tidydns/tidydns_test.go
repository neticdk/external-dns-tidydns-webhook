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

package tidydns

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.opentelemetry.io/otel/metric/noop"
)

func mockCounter(method, url string, code int) {
	// Do nothings
}

func TestNewTidyDnsClient(t *testing.T) {
	meter := noop.NewMeterProvider().Meter("test")
	client, err := NewTidyDnsClient("http://example.com", "user", "pass", (10 * time.Second), meter)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if client == nil {
		t.Fatalf("Expected client, got nil")
	}
}

func TestNewTidyDnsClientErrBadMeter(t *testing.T) {
	meter := &badMeter{}
	_, err := NewTidyDnsClient("http://example.com", "user", "pass", (10 * time.Second), meter)
	if err == nil {
		t.Fatalf("Expected an error, got nil")
	}
}

func TestListZones(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[{"id": "1", "name": "zone1"}, {"id": "2", "name": "zone2"}]`))
	}
	server := httptest.NewServer(http.HandlerFunc(handler))
	defer server.Close()

	client := &tidyDNSClient{
		client:   server.Client(),
		baseURL:  server.URL,
		username: "user",
		password: "pass",
		counter:  mockCounter,
	}

	zones, err := client.ListZones()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(zones) != 2 {
		t.Fatalf("Expected 2 zones, got %d", len(zones))
	}
}

func TestCreateRecord(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}
	server := httptest.NewServer(http.HandlerFunc(handler))
	defer server.Close()

	client := &tidyDNSClient{
		client:   server.Client(),
		baseURL:  server.URL,
		username: "user",
		password: "pass",
		counter:  mockCounter,
	}

	record := &Record{
		Type:        "A",
		Name:        "test",
		Description: "Test record",
		Destination: "1.2.3.4",
		TTL:         "300",
	}

	err := client.CreateRecord("1", record)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
}

func TestCreateRecordFailure(t *testing.T) {
	client := &tidyDNSClient{}
	record := &Record{
		Type:        "a",
		Name:        "test",
		Description: "Test record",
		Destination: "1.2.3.4",
		TTL:         "300",
	}

	err := client.CreateRecord("1", record)
	if err == nil {
		t.Fatalf("Expected error, got nil")
	}
}

func TestListRecords(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[{"id": "1", "type_name": "A", "name": "test", "description": "Test record", "destination": "1.2.3.4", "ttl": "300", "zone_name": "example.com", "zone_id": "1"}]`))
	}
	server := httptest.NewServer(http.HandlerFunc(handler))
	defer server.Close()

	client := &tidyDNSClient{
		client:   server.Client(),
		baseURL:  server.URL,
		username: "user",
		password: "pass",
		counter:  mockCounter,
	}

	records, err := client.ListRecords("1")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(records) != 1 {
		t.Fatalf("Expected 1 record, got %d", len(records))
	}
}

func TestDeleteRecord(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}
	server := httptest.NewServer(http.HandlerFunc(handler))
	defer server.Close()

	client := &tidyDNSClient{
		client:   server.Client(),
		baseURL:  server.URL,
		username: "user",
		password: "pass",
		counter:  mockCounter,
	}

	err := client.DeleteRecord("1", "1")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
}

func TestRequestErrBadRequest(t *testing.T) {
	client := &tidyDNSClient{
		baseURL: "http://example.com",
	}

	err := client.request("GET", "/tes\t", nil, nil)
	if err == nil {
		t.Fatalf("Expected error, got nil")
	}
}

func TestRequestErrorHandling(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}
	server := httptest.NewServer(http.HandlerFunc(handler))
	defer server.Close()

	client := &tidyDNSClient{
		client:   server.Client(),
		baseURL:  server.URL,
		username: "user",
		password: "pass",
		counter:  mockCounter,
	}

	err := client.request("GET", "/test", nil, nil)
	if err == nil {
		t.Fatalf("Expected error, got nil")
	}
}

func TestEncodeRecordType(t *testing.T) {
	tests := []struct {
		input    string
		expected RecordType
		err      error
	}{
		{"AAAA", RecordTypeA, nil},
		{"A", RecordTypeA, nil},
		{"CNAME", RecordTypeCNAME, nil},
		{"TXT", RecordTypeTXT, nil},
		{"UNKNOWN", RecordType(0), errors.New("unmapped record type UNKNOWN")},
	}

	for _, test := range tests {
		result, err := encodeRecordType(test.input)
		if result != test.expected || (err != nil && err.Error() != test.err.Error()) {
			t.Errorf("Expected %v and %v, got %v and %v", test.expected, test.err, result, err)
		}
	}
}
