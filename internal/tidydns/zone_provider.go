package tidydns

import (
	"context"
	"log/slog"
	"sync"
	"time"

	gotidydns "github.com/neticdk/tidydns-go/pkg/tidydns"
	"sigs.k8s.io/external-dns/endpoint"
)

// zoneRefreshTimeout is the maximum time allowed for a single ListZones call.
const zoneRefreshTimeout = 30 * time.Second

// ZoneProvider caches zone information and refreshes it at a configurable interval.
type ZoneProvider struct {
	mu           sync.RWMutex
	zones        []*gotidydns.ZoneInfo
	domainFilter endpoint.DomainFilter
	cancel       context.CancelFunc
	done         chan struct{}
	logger       *slog.Logger
}

// NewZoneProvider creates a zone provider that fetches zones initially and
// refreshes at the given interval. It blocks until the initial zone list is
// fetched. When domainFilter is configured only zones relevant to the filter
// are kept - this includes zones that match the filter (subdomains) and zones
// that are parents of filter entries (where records would be created).
// Call Stop to release resources when done.
func NewZoneProvider(client gotidydns.TidyDNSClient, updateInterval time.Duration, domainFilter endpoint.DomainFilter) (*ZoneProvider, error) {
	ctx, cancel := context.WithCancel(context.Background())

	fetchCtx, fetchCancel := context.WithTimeout(ctx, zoneRefreshTimeout)
	defer fetchCancel()

	zones, err := client.ListZones(fetchCtx)
	if err != nil {
		cancel()
		return nil, err
	}

	zp := &ZoneProvider{
		zones:        filterZones(zones, &domainFilter),
		domainFilter: domainFilter,
		cancel:       cancel,
		done:         make(chan struct{}),
		logger:       slog.Default(),
	}

	zp.logger.Info("zone provider initialized", "total", len(zones), "filtered", len(zp.zones))

	ticker := time.NewTicker(updateInterval)

	go func() {
		defer ticker.Stop()
		defer close(zp.done)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				refreshCtx, refreshCancel := context.WithTimeout(ctx, zoneRefreshTimeout)
				newZones, err := client.ListZones(refreshCtx)
				refreshCancel()
				if err != nil {
					zp.logger.Error("error updating zones", "error", err)
					continue
				}
				filtered := filterZones(newZones, &zp.domainFilter)
				zp.mu.Lock()
				zp.zones = filtered
				zp.mu.Unlock()
			}
		}
	}()

	return zp, nil
}

// filterZones returns only the zones relevant to the domain filter. If the
// filter is not configured all zones are returned. For plain filters a zone is
// kept when:
//   - Match(zone) is true: the zone name is a subdomain of (or equal to) a filter entry
//   - MatchParent(zone) is true: the zone is a parent of a filter entry (records for
//     the filter entry live in this zone)
//
// For regex filters zone-level filtering is skipped entirely because
// MatchParent cannot derive parent relationships from a regex pattern. The
// domain filter still handles record-level filtering so correctness is not
// affected - only the performance optimisation of reducing the zone set is lost.
func filterZones(zones []*gotidydns.ZoneInfo, df *endpoint.DomainFilter) []*gotidydns.ZoneInfo {
	if !df.IsConfigured() {
		return zones
	}
	// Regex filters: skip zone filtering, rely on record-level domain filter.
	if len(df.Filters) == 0 {
		return zones
	}
	var filtered []*gotidydns.ZoneInfo
	for _, zone := range zones {
		if df.Match(zone.Name) || df.MatchParent(zone.Name) {
			filtered = append(filtered, zone)
		}
	}
	return filtered
}

// Stop shuts down the background zone refresh goroutine and waits for it to exit.
func (zp *ZoneProvider) Stop() {
	zp.cancel()
	<-zp.done
}

// GetDomainFilter returns the configured domain filter.
func (zp *ZoneProvider) GetDomainFilter() endpoint.DomainFilter {
	return zp.domainFilter
}

// GetZones returns the current cached list of zones.
func (zp *ZoneProvider) GetZones() []*gotidydns.ZoneInfo {
	zp.mu.RLock()
	defer zp.mu.RUnlock()
	return zp.zones
}
