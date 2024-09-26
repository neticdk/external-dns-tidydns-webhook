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
	"fmt"
	"testing"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
)

func TestCounterProvider(t *testing.T) {
	meter := noop.NewMeterProvider().Meter("test")

	counter, err := counterProvider(meter, "test_counter", "Test counter description")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if counter == nil {
		t.Fatalf("Expected a valid counter function, got nil")
	}

	// Test the counter function
	counter("GET", "/test", 200)
}

type badMeter struct {
	noop.Meter
}

func (m *badMeter) Int64Counter(name string, options ...metric.Int64CounterOption) (metric.Int64Counter, error) {
	return nil, fmt.Errorf("error")
}

func TestCounterProviderError(t *testing.T) {
	meter := &badMeter{}
	_, err := counterProvider(meter, "test_counter", "Test counter description")

	if err == nil {
		t.Fatalf("Expected an error, got nil")
	}
}
