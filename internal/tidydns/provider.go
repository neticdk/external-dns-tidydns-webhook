package tidydns

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/neticdk/go-common/pkg/log"
	gotidydns "github.com/neticdk/tidydns-go/pkg/tidydns"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/net/idna"
	"golang.org/x/sync/errgroup"
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
	"sigs.k8s.io/external-dns/provider"
)

// zoneRecord wraps a RecordInfo with zone context since the tidydns-go library
// does not include zone information on records.
type zoneRecord struct {
	gotidydns.RecordInfo
	ZoneID   int
	ZoneName string
}

// defaultMaxConcurrency is used when no explicit concurrency limit is configured.
const defaultMaxConcurrency = 10

// Provider implements the external-dns provider.Provider interface for TidyDNS.
type Provider struct {
	provider.BaseProvider
	client         gotidydns.TidyDNSClient
	zoneProvider   *ZoneProvider
	maxConcurrency int

	tracer                  trace.Tracer
	recordsGauge            metric.Int64Gauge
	changesCounter          metric.Int64Counter
	recordsLatencyHist      metric.Float64Histogram
	applyChangesLatencyHist metric.Float64Histogram
}

const (
	instrumentationScope = "external-dns-tidydns-webhook"
	metricsNamespace     = "external_dns.webhook.tidydns"
)

// NewProvider creates a new TidyDNS provider.
func NewProvider(client gotidydns.TidyDNSClient, cfg Config) (*Provider, error) {
	zp, err := NewZoneProvider(client, cfg.ZoneUpdateInterval, cfg.DomainFilter)
	if err != nil {
		return nil, fmt.Errorf("initializing zone provider: %w", err)
	}

	maxConc := cfg.MaxConcurrency
	if maxConc <= 0 {
		maxConc = defaultMaxConcurrency
	}

	p, err := newInstrumentedProvider(client, zp, maxConc)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func newInstrumentedProvider(client gotidydns.TidyDNSClient, zp *ZoneProvider, maxConcurrency int) (*Provider, error) {
	tracer := otel.Tracer(instrumentationScope)
	meter := otel.Meter(instrumentationScope)

	recordsGauge, err := meter.Int64Gauge(metricsNamespace+".records",
		metric.WithDescription("Number of DNS record endpoints returned"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("creating records gauge: %w", err)
	}

	changesCounter, err := meter.Int64Counter(metricsNamespace+".changes",
		metric.WithDescription("Number of DNS record changes applied"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("creating changes counter: %w", err)
	}

	recordsLatencyHist, err := meter.Float64Histogram(metricsNamespace+".records.duration",
		metric.WithDescription("Duration of records listing operations"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, fmt.Errorf("creating records latency histogram: %w", err)
	}

	applyChangesLatencyHist, err := meter.Float64Histogram(metricsNamespace+".apply_changes.duration",
		metric.WithDescription("Duration of apply changes operations"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, fmt.Errorf("creating apply_changes latency histogram: %w", err)
	}

	return &Provider{
		client:                  client,
		zoneProvider:            zp,
		maxConcurrency:          maxConcurrency,
		tracer:                  tracer,
		recordsGauge:            recordsGauge,
		changesCounter:          changesCounter,
		recordsLatencyHist:      recordsLatencyHist,
		applyChangesLatencyHist: applyChangesLatencyHist,
	}, nil
}

// Stop releases resources held by the provider.
func (p *Provider) Stop() {
	p.zoneProvider.Stop()
}

// GetDomainFilter returns a domain filter for external-dns negotiation. When a
// domain filter is configured it is returned directly so that external-dns
// scopes record management to exactly those domains (which may be narrower than
// the TidyDNS zones). When no filter is configured the zone names are returned
// so that external-dns knows which zones the webhook manages.
func (p *Provider) GetDomainFilter() endpoint.DomainFilterInterface {
	if df := p.zoneProvider.GetDomainFilter(); df.IsConfigured() {
		return &df
	}
	zoneNames := make([]string, 0, len(p.zoneProvider.GetZones()))
	for _, zone := range p.zoneProvider.GetZones() {
		zoneNames = append(zoneNames, zone.Name)
	}
	return endpoint.NewDomainFilter(zoneNames)
}

// Records returns all DNS records from TidyDNS as external-dns endpoints.
// Multiple TidyDNS records with the same name and type are merged into a single
// endpoint with multiple targets.
func (p *Provider) Records(ctx context.Context) ([]*endpoint.Endpoint, error) {
	start := time.Now()
	ctx, span := p.tracer.Start(ctx, "tidydns.list_records", trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()

	log.FromContext(ctx).InfoContext(ctx, "listing DNS records")

	records, err := p.fetchAllRecords(ctx)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		p.recordsLatencyHist.Record(ctx, time.Since(start).Seconds())
		return nil, fmt.Errorf("listing records: %w", err)
	}

	type groupKey struct {
		name  string
		rType string
	}

	grouped := make(map[groupKey]*endpoint.Endpoint)

	for _, record := range records {
		ep := parseTidyRecord(&record)
		if ep == nil {
			continue
		}

		key := groupKey{name: ep.DNSName, rType: ep.RecordType}
		if existing, ok := grouped[key]; ok {
			existing.Targets = append(existing.Targets, ep.Targets...)
		} else {
			grouped[key] = ep
		}
	}

	endpoints := make([]*endpoint.Endpoint, 0, len(grouped))
	for _, ep := range grouped {
		endpoints = append(endpoints, ep)
	}

	p.recordsGauge.Record(ctx, int64(len(endpoints)))
	p.recordsLatencyHist.Record(ctx, time.Since(start).Seconds())
	span.SetAttributes(attribute.Int("dns.records.count", len(endpoints)))

	return endpoints, nil
}

// AdjustEndpoints adjusts endpoints to match TidyDNS constraints:
// - TTL is clamped to TidyDNS minimum (300s, or 0 for zone default)
// - Labels are removed (not supported by TidyDNS)
// - Unicode names are encoded as punycode
// - Unsupported record types (e.g. AAAA) are filtered out
func (p *Provider) AdjustEndpoints(endpoints []*endpoint.Endpoint) ([]*endpoint.Endpoint, error) {
	adjusted := make([]*endpoint.Endpoint, 0, len(endpoints))
	for _, ep := range endpoints {
		if !isSupportedRecordType(ep.RecordType) {
			// slog.Debug("filtering unsupported record type", "name", ep.DNSName, "type", ep.RecordType)
			continue
		}
		ep.RecordTTL = endpoint.TTL(clampTTL(int(ep.RecordTTL)))
		ep.Labels = endpoint.Labels{}
		ep.DNSName, _ = idna.Lookup.ToASCII(ep.DNSName)
		adjusted = append(adjusted, ep)
	}
	return adjusted, nil
}

// ApplyChanges deletes, updates, and creates DNS records in TidyDNS.
// Each phase runs sequentially (deletes first, then updates, then creates)
// with bounded parallelism within each phase.
func (p *Provider) ApplyChanges(ctx context.Context, changes *plan.Changes) error {
	if !changes.HasChanges() {
		return nil
	}

	start := time.Now()
	ctx, span := p.tracer.Start(ctx, "tidydns.apply_changes", trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()

	log.FromContext(ctx).InfoContext(ctx, "applying DNS changes",
		"creates", len(changes.Create),
		"deletes", len(changes.Delete),
		"updates", len(changes.UpdateOld),
	)

	span.SetAttributes(
		attribute.Int("dns.changes.create", len(changes.Create)),
		attribute.Int("dns.changes.delete", len(changes.Delete)),
		attribute.Int("dns.changes.update", len(changes.UpdateOld)),
	)

	zones := p.zoneProvider.GetZones()

	// Deletes
	if len(changes.Delete) > 0 {
		g, gctx := errgroup.WithContext(ctx)
		g.SetLimit(p.maxConcurrency)
		for _, ep := range changes.Delete {
			g.Go(func() error {
				return p.deleteEndpoint(gctx, zones, ep)
			})
		}
		if err := g.Wait(); err != nil {
			span.SetStatus(codes.Error, err.Error())
			p.applyChangesLatencyHist.Record(ctx, time.Since(start).Seconds())
			return err
		}
	}

	// Updates
	if len(changes.UpdateOld) > 0 {
		g, gctx := errgroup.WithContext(ctx)
		g.SetLimit(p.maxConcurrency)
		for i := range changes.UpdateOld {
			g.Go(func() error {
				return p.updateEndpoint(gctx, zones, changes.UpdateOld[i], changes.UpdateNew[i])
			})
		}
		if err := g.Wait(); err != nil {
			span.SetStatus(codes.Error, err.Error())
			p.applyChangesLatencyHist.Record(ctx, time.Since(start).Seconds())
			return err
		}
	}

	// Creates
	if len(changes.Create) > 0 {
		g, gctx := errgroup.WithContext(ctx)
		g.SetLimit(p.maxConcurrency)
		for _, ep := range changes.Create {
			g.Go(func() error {
				return p.createRecord(gctx, zones, ep)
			})
		}
		if err := g.Wait(); err != nil {
			span.SetStatus(codes.Error, err.Error())
			p.applyChangesLatencyHist.Record(ctx, time.Since(start).Seconds())
			return err
		}
	}

	p.changesCounter.Add(ctx, int64(len(changes.Create)), metric.WithAttributes(attribute.String("change_type", "create")))
	p.changesCounter.Add(ctx, int64(len(changes.Delete)), metric.WithAttributes(attribute.String("change_type", "delete")))
	p.changesCounter.Add(ctx, int64(len(changes.UpdateOld)), metric.WithAttributes(attribute.String("change_type", "update")))
	p.applyChangesLatencyHist.Record(ctx, time.Since(start).Seconds())

	return nil
}

// fetchAllRecords fetches records from all zones and wraps them with zone context.
func (p *Provider) fetchAllRecords(ctx context.Context) ([]zoneRecord, error) {
	var result []zoneRecord

	for _, zone := range p.zoneProvider.GetZones() {
		ctx, span := p.tracer.Start(ctx, "tidydns.list_records",
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(attribute.Int("tidydns.zone.id", zone.ID), attribute.String("tidydns.zone.name", zone.Name)),
		)
		records, err := p.client.ListRecords(ctx, zone.ID)
		if err != nil {
			span.SetStatus(codes.Error, err.Error())
			span.End()
			return nil, err
		}
		span.SetAttributes(attribute.Int("tidydns.records.count", len(records)))
		span.End()
		for _, r := range records {
			result = append(result, zoneRecord{
				RecordInfo: *r,
				ZoneID:     zone.ID,
				ZoneName:   zone.Name,
			})
		}
	}

	return result, nil
}

func (p *Provider) deleteEndpoint(ctx context.Context, zones []*gotidydns.ZoneInfo, ep *endpoint.Endpoint) error {
	ctx, span := p.tracer.Start(ctx, "tidydns.delete_record",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attribute.String("dns.name", ep.DNSName), attribute.String("dns.record.type", ep.RecordType)),
	)
	defer span.End()

	logger := log.FromContext(ctx)

	recordType, ok := stringToRecordType(ep.RecordType)
	if !ok {
		logger.WarnContext(ctx, "skipping delete of unsupported record type", "name", ep.DNSName, "type", ep.RecordType)
		return nil
	}

	dnsName, zoneID := tidyfyName(zones, ep.DNSName)
	if dnsName == "" {
		logger.DebugContext(ctx, "skipping delete, no matching zone", "name", ep.DNSName)
		return nil
	}

	records, err := p.findRecords(ctx, zoneID, dnsName, recordType)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("finding records for delete %s (%s): %w", ep.DNSName, ep.RecordType, err)
	}

	for _, target := range ep.Targets {
		target = tidyfyTarget(target)
		for _, record := range records {
			if record.Destination != target {
				continue
			}
			logger.DebugContext(ctx, "deleting record", "name", dnsName, "type", ep.RecordType, "destination", record.Destination)
			if err := p.deleteRecord(ctx, zoneID, record.ID); err != nil {
				span.SetStatus(codes.Error, err.Error())
				return fmt.Errorf("deleting record %s (%s): %w", dnsName, ep.RecordType, err)
			}
		}
	}
	return nil
}

// updateEndpoint updates existing records in place using the TidyDNS
// UpdateRecord API. For each old target it finds the matching TidyDNS record
// and updates it with the corresponding new target. If old and new have
// different target counts, extra old targets are deleted and extra new targets
// are created.
func (p *Provider) updateEndpoint(ctx context.Context, zones []*gotidydns.ZoneInfo, oldEp, newEp *endpoint.Endpoint) error {
	ctx, span := p.tracer.Start(ctx, "tidydns.update_record",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attribute.String("dns.name", oldEp.DNSName), attribute.String("dns.record.type", oldEp.RecordType)),
	)
	defer span.End()

	logger := log.FromContext(ctx)

	recordType, ok := stringToRecordType(oldEp.RecordType)
	if !ok {
		logger.WarnContext(ctx, "skipping update of unsupported record type", "name", oldEp.DNSName, "type", oldEp.RecordType)
		return nil
	}

	oldName, oldZoneID := tidyfyName(zones, oldEp.DNSName)
	if oldName == "" {
		logger.DebugContext(ctx, "skipping update, no matching zone for old endpoint", "name", oldEp.DNSName)
		return nil
	}

	records, err := p.findRecords(ctx, oldZoneID, oldName, recordType)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("finding records for update %s (%s): %w", oldEp.DNSName, oldEp.RecordType, err)
	}

	// Match old targets to existing TidyDNS records.
	var matched []*gotidydns.RecordInfo
	for _, target := range oldEp.Targets {
		target = tidyfyTarget(target)
		for _, record := range records {
			if record.Destination == target {
				matched = append(matched, record)
				break
			}
		}
	}

	newName, newZoneID := tidyfyName(zones, newEp.DNSName)
	if newName == "" {
		logger.DebugContext(ctx, "skipping update, no matching zone for new endpoint", "name", newEp.DNSName)
		return nil
	}

	newRecordType, ok := stringToRecordType(newEp.RecordType)
	if !ok {
		logger.WarnContext(ctx, "skipping update, unsupported new record type", "name", newEp.DNSName, "type", newEp.RecordType)
		return nil
	}

	ttl := clampTTL(int(newEp.RecordTTL))

	// Update records that exist in both old and new.
	minLen := len(matched)
	if len(newEp.Targets) < minLen {
		minLen = len(newEp.Targets)
	}

	for i := 0; i < minLen; i++ {
		target := tidyfyTarget(newEp.Targets[i])
		if newEp.RecordType == RecordTypeCNAME {
			target += "."
		}

		info := gotidydns.RecordInfo{
			Type:        newRecordType,
			Name:        newName,
			Description: newEp.SetIdentifier,
			Destination: target,
			TTL:         ttl,
			Location:    recordLocation(newEp.RecordType, target),
		}

		logger.DebugContext(ctx, "updating record", "name", newName, "type", newEp.RecordType, "destination", target, "recordID", matched[i].ID)
		if err := p.updateRecord(ctx, oldZoneID, matched[i].ID, info); err != nil {
			span.SetStatus(codes.Error, err.Error())
			return fmt.Errorf("updating record %s (%s): %w", newName, newEp.RecordType, err)
		}
	}

	// Delete extra old targets that have no corresponding new target.
	for i := minLen; i < len(matched); i++ {
		logger.DebugContext(ctx, "deleting extra record during update", "name", oldName, "recordID", matched[i].ID)
		if err := p.deleteRecord(ctx, oldZoneID, matched[i].ID); err != nil {
			span.SetStatus(codes.Error, err.Error())
			return fmt.Errorf("deleting record %s (%s): %w", oldName, oldEp.RecordType, err)
		}
	}

	// Create extra new targets that have no corresponding old record.
	for i := minLen; i < len(newEp.Targets); i++ {
		target := tidyfyTarget(newEp.Targets[i])
		if newEp.RecordType == RecordTypeCNAME {
			target += "."
		}

		info := gotidydns.RecordInfo{
			Type:        newRecordType,
			Name:        newName,
			Description: newEp.SetIdentifier,
			Destination: target,
			TTL:         ttl,
			Location:    recordLocation(newEp.RecordType, target),
		}

		logger.DebugContext(ctx, "creating extra record during update", "name", newName, "type", newEp.RecordType, "destination", target)
		if err := p.createSingleRecord(ctx, newZoneID, info); err != nil {
			span.SetStatus(codes.Error, err.Error())
			return fmt.Errorf("creating record %s (%s): %w", newName, newEp.RecordType, err)
		}
	}

	return nil
}

func (p *Provider) createRecord(ctx context.Context, zones []*gotidydns.ZoneInfo, ep *endpoint.Endpoint) error {
	logger := log.FromContext(ctx)

	recordType, ok := stringToRecordType(ep.RecordType)
	if !ok {
		logger.WarnContext(ctx, "skipping create of unsupported record type", "name", ep.DNSName, "type", ep.RecordType)
		return nil
	}

	dnsName, zoneID := tidyfyName(zones, ep.DNSName)
	if dnsName == "" {
		logger.DebugContext(ctx, "skipping create, no matching zone", "name", ep.DNSName)
		return nil
	}

	ttl := clampTTL(int(ep.RecordTTL))

	for _, target := range ep.Targets {
		target = tidyfyTarget(target)

		if ep.RecordType == RecordTypeCNAME {
			target += "."
		}

		info := gotidydns.RecordInfo{
			Type:        recordType,
			Name:        dnsName,
			Description: ep.SetIdentifier,
			Destination: target,
			TTL:         ttl,
			Location:    recordLocation(ep.RecordType, target),
		}

		logger.DebugContext(ctx, "creating record", "name", dnsName, "type", ep.RecordType, "destination", target)
		if err := p.createSingleRecord(ctx, zoneID, info); err != nil {
			return fmt.Errorf("creating record %s (%s): %w", dnsName, ep.RecordType, err)
		}
	}
	return nil
}

// parseTidyRecord converts a zoneRecord into an external-dns Endpoint.
func parseTidyRecord(record *zoneRecord) *endpoint.Endpoint {
	recordType := recordTypeToString(record.Type)
	dnsName := tidyNameToFQDN(record.Name, record.ZoneName)
	if recordType == "" {
		// slog.Debug("skipping unsupported record type", "type", recordType, "name", dnsName)
		return nil
	}

	ttl := endpoint.TTL(record.TTL)

	destination := record.Destination
	if recordType == RecordTypeCNAME {
		destination = strings.TrimRight(destination, ".")
	}

	ep := endpoint.NewEndpointWithTTL(dnsName, recordType, ttl, destination)
	ep.SetIdentifier = record.Description
	return ep
}

// findRecords wraps client.FindRecord with a child span.
func (p *Provider) findRecords(ctx context.Context, zoneID int, name string, rType gotidydns.RecordType) ([]*gotidydns.RecordInfo, error) {
	ctx, span := p.tracer.Start(ctx, "tidydns.find_record",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attribute.Int("tidydns.zone.id", zoneID), attribute.String("tidydns.record.name", name)),
	)
	defer span.End()

	records, err := p.client.FindRecord(ctx, zoneID, name, rType)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	span.SetAttributes(attribute.Int("tidydns.records.count", len(records)))
	return records, nil
}

// deleteRecord wraps client.DeleteRecord with a child span.
func (p *Provider) deleteRecord(ctx context.Context, zoneID, recordID int) error {
	ctx, span := p.tracer.Start(ctx, "tidydns.api.delete_record",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attribute.Int("tidydns.zone.id", zoneID), attribute.Int("tidydns.record.id", recordID)),
	)
	defer span.End()

	if err := p.client.DeleteRecord(ctx, zoneID, recordID); err != nil {
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

// updateRecord wraps client.UpdateRecord with a child span.
func (p *Provider) updateRecord(ctx context.Context, zoneID, recordID int, info gotidydns.RecordInfo) error {
	ctx, span := p.tracer.Start(ctx, "tidydns.api.update_record",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attribute.Int("tidydns.zone.id", zoneID), attribute.Int("tidydns.record.id", recordID)),
	)
	defer span.End()

	if err := p.client.UpdateRecord(ctx, zoneID, recordID, info); err != nil {
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

// createSingleRecord wraps client.CreateRecord with a child span.
func (p *Provider) createSingleRecord(ctx context.Context, zoneID int, info gotidydns.RecordInfo) error {
	ctx, span := p.tracer.Start(ctx, "tidydns.api.create_record",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attribute.Int("tidydns.zone.id", zoneID), attribute.String("tidydns.record.name", info.Name)),
	)
	defer span.End()

	id, err := p.client.CreateRecord(ctx, zoneID, info)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	span.SetAttributes(attribute.Int("tidydns.record.id", id))
	return nil
}
