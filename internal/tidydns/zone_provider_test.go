package tidydns

import (
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/neticdk/go-stdlib/assert"
	"github.com/neticdk/tidydns-go/pkg/tidydns"
	"github.com/stretchr/testify/mock"
	"sigs.k8s.io/external-dns/endpoint"
)

func TestNewZoneProvider(t *testing.T) {
	client := &mockClient{}
	zones := []*tidydns.ZoneInfo{{ID: 1, Name: "example.com"}}
	client.On("ListZones", mock.Anything).Return(zones, nil)

	zp, err := NewZoneProvider(client, 1*time.Hour, endpoint.DomainFilter{})
	assert.NoError(t, err)
	assert.NotNil(t, zp)
	t.Cleanup(func() { zp.Stop() })

	got := zp.GetZones()
	assert.Equal(t, got, zones)
}

func TestNewZoneProviderListZonesError(t *testing.T) {
	client := &mockClient{}
	client.On("ListZones", mock.Anything).Return(([]*tidydns.ZoneInfo)(nil), fmt.Errorf("connection refused"))

	zp, err := NewZoneProvider(client, 1*time.Hour, endpoint.DomainFilter{})
	assert.Error(t, err)
	assert.Nil(t, zp)
}

func TestZoneProviderGetZonesAfterStop(t *testing.T) {
	client := &mockClient{}
	zones := []*tidydns.ZoneInfo{{ID: 1, Name: "example.com"}}
	client.On("ListZones", mock.Anything).Return(zones, nil)

	zp, err := NewZoneProvider(client, 1*time.Hour, endpoint.DomainFilter{})
	assert.NoError(t, err)

	zp.Stop()

	// GetZones after Stop should not deadlock - returns last cached value
	got := zp.GetZones()
	assert.Equal(t, got, zones)
}

func TestZoneProviderRefresh(t *testing.T) {
	client := &mockClient{}
	initial := []*tidydns.ZoneInfo{{ID: 1, Name: "old.com"}}
	updated := []*tidydns.ZoneInfo{{ID: 1, Name: "old.com"}, {ID: 2, Name: "new.com"}}

	client.On("ListZones", mock.Anything).Return(initial, nil).Once()
	client.On("ListZones", mock.Anything).Return(updated, nil).Maybe()

	zp, err := NewZoneProvider(client, 50*time.Millisecond, endpoint.DomainFilter{})
	assert.NoError(t, err)
	t.Cleanup(func() { zp.Stop() })

	// Wait for at least one refresh cycle
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(zp.GetZones()) == 2 {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	assert.Len(t, zp.GetZones(), 2)
}

func TestZoneProviderDomainFilter(t *testing.T) {
	client := &mockClient{}
	allZones := []*tidydns.ZoneInfo{
		{ID: 1, Name: "example.com"},
		{ID: 2, Name: "other.com"},
		{ID: 3, Name: "sub.example.com"},
		{ID: 4, Name: "unrelated.org"},
	}
	client.On("ListZones", mock.Anything).Return(allZones, nil)

	zp, err := NewZoneProvider(client, 1*time.Hour, *endpoint.NewDomainFilter([]string{"example.com"}))
	assert.NoError(t, err)
	t.Cleanup(func() { zp.Stop() })

	got := zp.GetZones()
	assert.Len(t, got, 2)
	names := zoneNames(got)
	assert.ElementsMatch(t, names, []string{"example.com", "sub.example.com"})
}

func TestZoneProviderDomainFilterMultiple(t *testing.T) {
	client := &mockClient{}
	allZones := []*tidydns.ZoneInfo{
		{ID: 1, Name: "example.com"},
		{ID: 2, Name: "other.com"},
		{ID: 3, Name: "corp.net"},
	}
	client.On("ListZones", mock.Anything).Return(allZones, nil)

	zp, err := NewZoneProvider(client, 1*time.Hour, *endpoint.NewDomainFilter([]string{"example.com", "corp.net"}))
	assert.NoError(t, err)
	t.Cleanup(func() { zp.Stop() })

	got := zp.GetZones()
	assert.Len(t, got, 2)
	names := zoneNames(got)
	assert.ElementsMatch(t, names, []string{"example.com", "corp.net"})
}

func TestZoneProviderExcludeDomains(t *testing.T) {
	client := &mockClient{}
	allZones := []*tidydns.ZoneInfo{
		{ID: 1, Name: "example.com"},
		{ID: 2, Name: "staging.example.com"},
		{ID: 3, Name: "other.com"},
	}
	client.On("ListZones", mock.Anything).Return(allZones, nil)

	df := *endpoint.NewDomainFilterWithExclusions([]string{"example.com"}, []string{"staging.example.com"})
	zp, err := NewZoneProvider(client, 1*time.Hour, df)
	assert.NoError(t, err)
	t.Cleanup(func() { zp.Stop() })

	got := zp.GetZones()
	names := zoneNames(got)
	assert.ElementsMatch(t, names, []string{"example.com"})
}

func TestZoneProviderRegexFilter(t *testing.T) {
	client := &mockClient{}
	allZones := []*tidydns.ZoneInfo{
		{ID: 1, Name: "prod.example.com"},
		{ID: 2, Name: "staging.example.com"},
		{ID: 3, Name: "other.com"},
	}
	client.On("ListZones", mock.Anything).Return(allZones, nil)

	df := *endpoint.NewRegexDomainFilter(regexp.MustCompile(`\.example\.com$`), nil)
	zp, err := NewZoneProvider(client, 1*time.Hour, df)
	assert.NoError(t, err)
	t.Cleanup(func() { zp.Stop() })

	// Regex filters skip zone filtering; all zones are returned.
	// Record-level domain filter handles correctness.
	got := zp.GetZones()
	names := zoneNames(got)
	assert.ElementsMatch(t, names, []string{"prod.example.com", "staging.example.com", "other.com"})
}

func TestZoneProviderRegexWithExclusion(t *testing.T) {
	client := &mockClient{}
	allZones := []*tidydns.ZoneInfo{
		{ID: 1, Name: "prod.example.com"},
		{ID: 2, Name: "staging.example.com"},
		{ID: 3, Name: "other.com"},
	}
	client.On("ListZones", mock.Anything).Return(allZones, nil)

	df := *endpoint.NewRegexDomainFilter(
		regexp.MustCompile(`\.example\.com$`),
		regexp.MustCompile(`^staging\.`),
	)
	zp, err := NewZoneProvider(client, 1*time.Hour, df)
	assert.NoError(t, err)
	t.Cleanup(func() { zp.Stop() })

	// Regex filters skip zone filtering; all zones are returned.
	got := zp.GetZones()
	names := zoneNames(got)
	assert.ElementsMatch(t, names, []string{"prod.example.com", "staging.example.com", "other.com"})
}

func TestFilterZones(t *testing.T) {
	zones := []*tidydns.ZoneInfo{
		{ID: 1, Name: "example.com"},
		{ID: 2, Name: "sub.example.com"},
		{ID: 3, Name: "other.com"},
		{ID: 4, Name: "notexample.com"},
		{ID: 5, Name: "k8s.example.com"},
	}

	tests := []struct {
		name string
		df   endpoint.DomainFilter
		want []string
	}{
		{"unconfigured filter returns all", endpoint.DomainFilter{}, []string{"example.com", "sub.example.com", "other.com", "notexample.com", "k8s.example.com"}},
		{"exact match", *endpoint.NewDomainFilter([]string{"other.com"}), []string{"other.com"}},
		{"zone is subdomain of filter", *endpoint.NewDomainFilter([]string{"example.com"}), []string{"example.com", "sub.example.com", "k8s.example.com"}},
		{"filter is subdomain of zone (parent kept)", *endpoint.NewDomainFilter([]string{"app.sub.k8s.example.com"}), []string{"example.com", "k8s.example.com"}},
		{"no suffix false positive", *endpoint.NewDomainFilter([]string{"example.com"}), []string{"example.com", "sub.example.com", "k8s.example.com"}},
		{"no match", *endpoint.NewDomainFilter([]string{"missing.com"}), nil},
		{"mixed: filter narrower and broader", *endpoint.NewDomainFilter([]string{"deep.sub.example.com", "other.com"}), []string{"example.com", "sub.example.com", "other.com"}},
		{"with exclusion", *endpoint.NewDomainFilterWithExclusions([]string{"example.com"}, []string{"sub.example.com"}), []string{"example.com", "k8s.example.com"}},
		{"regex skips zone filter", *endpoint.NewRegexDomainFilter(regexp.MustCompile(`^other\.com$`), nil), []string{"example.com", "sub.example.com", "other.com", "notexample.com", "k8s.example.com"}},
		{"regex parent skips zone filter", *endpoint.NewRegexDomainFilter(regexp.MustCompile(`^deep\.sub\.example\.com$`), nil), []string{"example.com", "sub.example.com", "other.com", "notexample.com", "k8s.example.com"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterZones(zones, &tt.df)
			names := zoneNames(got)
			if tt.want == nil {
				assert.Empty(t, got)
			} else {
				assert.ElementsMatch(t, names, tt.want)
			}
		})
	}
}

func zoneNames(zones []*tidydns.ZoneInfo) []string {
	names := make([]string, len(zones))
	for i, z := range zones {
		names[i] = z.Name
	}
	return names
}
