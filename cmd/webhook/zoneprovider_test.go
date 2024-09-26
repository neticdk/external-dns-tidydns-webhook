/*
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
	"errors"
	"testing"
	"time"

	"github.com/neticdk/external-dns-tidydns-webhook/cmd/webhook/tidydns"
)

func TestNewZoneProvider(t *testing.T) {
	mockZones := []tidydns.Zone{
		{Name: "zone1"},
		{Name: "zone2"},
	}

	mockClient := &mockTidyDNSClient{zones: mockZones}
	provider := newZoneProvider(mockClient, (10 * time.Minute))

	zones := provider.getZones()
	if len(zones) != len(mockZones) {
		t.Fatalf("Expected %d zones, got %d", len(mockZones), len(zones))
	}

	for i, zone := range zones {
		if zone.Name != mockZones[i].Name {
			t.Errorf("Expected zone name %s, got %s", mockZones[i].Name, zone.Name)
		}
	}
}

func TestZoneProviderUpdateWithError(t *testing.T) {
	initialZones := []tidydns.Zone{
		{Name: "zone1"},
	}

	mockClient := &mockTidyDNSClient{zones: initialZones}
	provider := newZoneProvider(mockClient, (1 * time.Second))

	// Initial zones check
	zones := provider.getZones()
	if len(zones) != len(initialZones) {
		t.Fatalf("Expected %d initial zones, got %d", len(initialZones), len(zones))
	}

	for i, zone := range zones {
		if zone.Name != initialZones[i].Name {
			t.Errorf("Expected initial zone name %s, got %s", initialZones[i].Name, zone.Name)
		}
	}

	// Introduce an error in the mock client
	mockClient.err = errors.New("mock update error")

	// Wait for the update interval to pass
	time.Sleep(2 * time.Second)

	// Check zones after error
	zones = provider.getZones()
	if len(zones) != len(initialZones) {
		t.Fatalf("Expected %d zones after error, got %d", len(initialZones), len(zones))
	}

	for i, zone := range zones {
		if zone.Name != initialZones[i].Name {
			t.Errorf("Expected zone name %s after error, got %s", initialZones[i].Name, zone.Name)
		}
	}
}

func TestZoneProviderUpdateWithNewZones(t *testing.T) {
	initialZones := []tidydns.Zone{
		{Name: "zone1"},
	}

	updatedZones := []tidydns.Zone{
		{Name: "zone1"},
		{Name: "zone2"},
	}

	mockClient := &mockTidyDNSClient{zones: initialZones}
	provider := newZoneProvider(mockClient, (1 * time.Second))

	// Initial zones check
	zones := provider.getZones()
	if len(zones) != len(initialZones) {
		t.Fatalf("Expected %d initial zones, got %d", len(initialZones), len(zones))
	}

	for i, zone := range zones {
		if zone.Name != initialZones[i].Name {
			t.Errorf("Expected initial zone name %s, got %s", initialZones[i].Name, zone.Name)
		}
	}

	// Update the zones in the mock client
	mockClient.zones = updatedZones

	// Wait for the update interval to pass
	time.Sleep(2 * time.Second)

	// Check zones after update
	zones = provider.getZones()
	if len(zones) != len(updatedZones) {
		t.Fatalf("Expected %d zones after update, got %d", len(updatedZones), len(zones))
	}

	for i, zone := range zones {
		if zone.Name != updatedZones[i].Name {
			t.Errorf("Expected zone name %s after update, got %s", updatedZones[i].Name, zone.Name)
		}
	}
}

func TestZoneProviderErrorHandling(t *testing.T) {
	mockClient := &mockTidyDNSClient{err: errors.New("mock error")}

	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected panic due to error in ListZones")
		}
	}()

	newZoneProvider(mockClient, (10 * time.Minute))
}

func TestZoneProviderNoZones(t *testing.T) {
	mockClient := &mockTidyDNSClient{zones: []tidydns.Zone{}}

	provider := newZoneProvider(mockClient, (10 * time.Minute))

	zones := provider.getZones()
	if len(zones) != 0 {
		t.Fatalf("Expected 0 zones, got %d", len(zones))
	}
}
