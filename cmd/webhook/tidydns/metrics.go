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
