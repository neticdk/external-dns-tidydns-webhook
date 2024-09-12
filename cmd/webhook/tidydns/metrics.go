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
	"context"

	"go.opentelemetry.io/otel/attribute"
	otel "go.opentelemetry.io/otel/metric"
)

type counter func(method, url string, code int)

func counterProvider(meter otel.Meter, name, desc string) (counter, error) {
	description := otel.WithDescription(desc)
	intCounter, err := meter.Int64Counter(name, description)
	if err != nil {
		return nil, err
	}

	count := func(method, url string, code int) {
		// Make counter labels
		opt := otel.WithAttributes(
			attribute.Key("method").String(method),
			attribute.Key("endpoint").String(url),
			attribute.Key("code").Int(code),
		)

		// Count new request response
		intCounter.Add(context.Background(), 1, opt)
	}

	return count, nil
}
