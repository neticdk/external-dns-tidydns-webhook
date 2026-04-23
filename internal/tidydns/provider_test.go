package tidydns

import (
	"context"
	"fmt"
	"testing"

	"github.com/neticdk/go-stdlib/assert"
	"github.com/neticdk/tidydns-go/pkg/tidydns"
	"github.com/stretchr/testify/mock"
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
)

var (
	testZones = []*tidydns.ZoneInfo{
		{ID: 1, Name: "example.com"},
		{ID: 2, Name: "other.com"},
		{ID: 3, Name: "sub.other.com"},
	}

	zone1Records = []*tidydns.RecordInfo{
		{ID: 11, Type: tidydns.RecordTypeA, Name: "www", Destination: "1.2.3.4", TTL: 300},
		{ID: 12, Type: tidydns.RecordTypeCNAME, Name: "alias", Destination: "www.example.com.", TTL: 300},
		{ID: 13, Type: tidydns.RecordTypeTXT, Name: "txt", Destination: "v=spf1 include:example.com", TTL: 300},
	}

	zone2Records = []*tidydns.RecordInfo{
		{ID: 21, Type: tidydns.RecordTypeA, Name: "app", Destination: "5.6.7.8", TTL: 600},
	}
)

// newTestProvider creates a Provider with the given mock client and a
// pre-populated ZoneProvider (no background goroutine).
func newTestProvider(t *testing.T, client *mockClient, zones []*tidydns.ZoneInfo) *Provider {
	t.Helper()
	client.On("ListZones", mock.Anything).Return(zones, nil).Maybe()

	zp, err := NewZoneProvider(client, 1<<62, endpoint.DomainFilter{}) // effectively never ticks
	if err != nil {
		t.Fatalf("NewZoneProvider: %v", err)
	}
	t.Cleanup(func() { zp.Stop() })

	p, err := newInstrumentedProvider(client, zp, defaultMaxConcurrency)
	if err != nil {
		t.Fatalf("newInstrumentedProvider: %v", err)
	}
	return p
}

func TestRecords(t *testing.T) {
	client := &mockClient{}
	client.On("ListRecords", mock.Anything, 1).Return(zone1Records, nil)
	client.On("ListRecords", mock.Anything, 2).Return(zone2Records, nil)
	client.On("ListRecords", mock.Anything, 3).Return([]*tidydns.RecordInfo{}, nil)

	p := newTestProvider(t, client, testZones)

	eps, err := p.Records(context.Background())
	assert.NoError(t, err)
	assert.Len(t, eps, 4) // www A, alias CNAME, txt TXT, app A
	client.AssertExpectations(t)
}

func TestRecordsMergesTargets(t *testing.T) {
	records := []*tidydns.RecordInfo{
		{ID: 1, Type: tidydns.RecordTypeA, Name: "multi", Destination: "1.1.1.1"},
		{ID: 2, Type: tidydns.RecordTypeA, Name: "multi", Destination: "2.2.2.2"},
	}

	client := &mockClient{}
	client.On("ListRecords", mock.Anything, 1).Return(records, nil)

	p := newTestProvider(t, client, []*tidydns.ZoneInfo{{ID: 1, Name: "example.com"}})

	eps, err := p.Records(context.Background())
	assert.NoError(t, err)
	assert.Len(t, eps, 1)
	assert.ElementsMatch(t, eps[0].Targets, []string{"1.1.1.1", "2.2.2.2"})
}

func TestApplyChangesCreate(t *testing.T) {
	client := &mockClient{}
	client.On("CreateRecord", mock.Anything, 1, tidydns.RecordInfo{
		Type: tidydns.RecordTypeA, Name: "new", Destination: "1.1.1.1",
	}).Return(1, nil)

	p := newTestProvider(t, client, []*tidydns.ZoneInfo{{ID: 1, Name: "example.com"}})

	changes := &plan.Changes{
		Create: []*endpoint.Endpoint{
			{DNSName: "new.example.com", Targets: endpoint.Targets{"1.1.1.1"}, RecordType: "A"},
		},
	}
	err := p.ApplyChanges(context.Background(), changes)
	assert.NoError(t, err)
	client.AssertExpectations(t)
}

func TestApplyChangesCreateCNAME(t *testing.T) {
	client := &mockClient{}
	client.On("CreateRecord", mock.Anything, 1, tidydns.RecordInfo{
		Type: tidydns.RecordTypeCNAME, Name: "alias", Destination: "target.example.com.",
	}).Return(1, nil)

	p := newTestProvider(t, client, []*tidydns.ZoneInfo{{ID: 1, Name: "example.com"}})

	changes := &plan.Changes{
		Create: []*endpoint.Endpoint{
			{DNSName: "alias.example.com", Targets: endpoint.Targets{"target.example.com"}, RecordType: "CNAME"},
		},
	}
	err := p.ApplyChanges(context.Background(), changes)
	assert.NoError(t, err)
	client.AssertExpectations(t)
}

func TestApplyChangesDelete(t *testing.T) {
	existing := []*tidydns.RecordInfo{
		{ID: 10, Type: tidydns.RecordTypeA, Name: "old", Destination: "1.1.1.1"},
	}

	client := &mockClient{}
	client.On("FindRecord", mock.Anything, 1, "old", tidydns.RecordTypeA).Return(existing, nil)
	client.On("DeleteRecord", mock.Anything, 1, 10).Return(nil)

	p := newTestProvider(t, client, []*tidydns.ZoneInfo{{ID: 1, Name: "example.com"}})

	changes := &plan.Changes{
		Delete: []*endpoint.Endpoint{
			{DNSName: "old.example.com", Targets: endpoint.Targets{"1.1.1.1"}, RecordType: "A"},
		},
	}
	err := p.ApplyChanges(context.Background(), changes)
	assert.NoError(t, err)
	client.AssertExpectations(t)
}

func TestApplyChangesUpdate(t *testing.T) {
	existing := []*tidydns.RecordInfo{
		{ID: 5, Type: tidydns.RecordTypeA, Name: "app", Destination: "1.0.0.1"},
	}

	client := &mockClient{}
	client.On("FindRecord", mock.Anything, 1, "app", tidydns.RecordTypeA).Return(existing, nil)
	client.On("UpdateRecord", mock.Anything, 1, 5, tidydns.RecordInfo{
		Type: tidydns.RecordTypeA, Name: "app", Destination: "2.0.0.2", TTL: 300,
	}).Return(nil)

	p := newTestProvider(t, client, []*tidydns.ZoneInfo{{ID: 1, Name: "example.com"}})

	changes := &plan.Changes{
		UpdateOld: []*endpoint.Endpoint{
			{DNSName: "app.example.com", Targets: endpoint.Targets{"1.0.0.1"}, RecordType: "A"},
		},
		UpdateNew: []*endpoint.Endpoint{
			{DNSName: "app.example.com", Targets: endpoint.Targets{"2.0.0.2"}, RecordType: "A", RecordTTL: 300},
		},
	}
	err := p.ApplyChanges(context.Background(), changes)
	assert.NoError(t, err)
	client.AssertExpectations(t)
}

func TestApplyChangesNoChanges(t *testing.T) {
	client := &mockClient{}
	p := newTestProvider(t, client, testZones)

	err := p.ApplyChanges(context.Background(), &plan.Changes{})
	assert.NoError(t, err)
	// No ListRecords calls expected
	client.AssertNotCalled(t, "ListRecords", mock.Anything, mock.Anything)
}

func TestGetDomainFilter(t *testing.T) {
	client := &mockClient{}
	p := newTestProvider(t, client, testZones)

	filter := p.GetDomainFilter()
	assert.True(t, filter.Match("example.com"))
	assert.True(t, filter.Match("other.com"))
	assert.True(t, filter.Match("sub.other.com"))
	assert.False(t, filter.Match("unknown.com"))
}

func TestGetDomainFilterWithConfiguredFilter(t *testing.T) {
	client := &mockClient{}
	// TidyDNS has a broad zone, but we only want to manage a subdomain of it
	allZones := []*tidydns.ZoneInfo{
		{ID: 1, Name: "k8s.example.com"},
		{ID: 2, Name: "other.com"},
	}
	client.On("ListZones", mock.Anything).Return(allZones, nil).Maybe()

	df := *endpoint.NewDomainFilter([]string{"app.k8s.example.com"})
	zp, err := NewZoneProvider(client, 1<<62, df)
	if err != nil {
		t.Fatalf("NewZoneProvider: %v", err)
	}
	t.Cleanup(func() { zp.Stop() })

	p, err := newInstrumentedProvider(client, zp, defaultMaxConcurrency)
	if err != nil {
		t.Fatalf("newInstrumentedProvider: %v", err)
	}

	// Zone k8s.example.com is kept (parent of filter entry)
	zones := zp.GetZones()
	assert.Len(t, zones, 1)
	assert.Equal(t, zones[0].Name, "k8s.example.com")

	// GetDomainFilter returns the configured filter, not zone names
	filter := p.GetDomainFilter()
	assert.True(t, filter.Match("app.k8s.example.com"))
	assert.True(t, filter.Match("deep.app.k8s.example.com"))
	assert.False(t, filter.Match("other.k8s.example.com"))
	assert.False(t, filter.Match("other.com"))
}

func TestAdjustEndpoints(t *testing.T) {
	client := &mockClient{}
	p := newTestProvider(t, client, testZones)

	endpoints := []*endpoint.Endpoint{
		{DNSName: "test.example.com", RecordType: "A", RecordTTL: 60},
		{DNSName: "test.example.com", RecordType: "AAAA", RecordTTL: 300},
		{DNSName: "test.example.com", RecordType: "CNAME", RecordTTL: 0},
		{DNSName: "münchen.example.com", RecordType: "A", RecordTTL: 300},
	}

	adjusted, err := p.AdjustEndpoints(endpoints)
	assert.NoError(t, err)
	assert.Len(t, adjusted, 3, "AAAA should be filtered")

	// TTL clamped from 60 to 300
	assert.Equal(t, adjusted[0].RecordTTL, endpoint.TTL(300))
	// TTL 0 stays 0 (zone default)
	assert.Equal(t, adjusted[1].RecordTTL, endpoint.TTL(0))
	// Unicode encoded to punycode
	assert.Equal(t, adjusted[2].DNSName, "xn--mnchen-3ya.example.com")
	// Labels cleared
	assert.Empty(t, adjusted[0].Labels)
}

func TestParseTidyRecord(t *testing.T) {
	tests := []struct {
		name     string
		record   zoneRecord
		wantNil  bool
		wantName string
		wantType string
		wantTgt  string
	}{
		{
			name: "A record",
			record: zoneRecord{
				RecordInfo: tidydns.RecordInfo{Type: tidydns.RecordTypeA, Name: "www", Destination: "1.2.3.4", TTL: 300},
				ZoneName:   "example.com",
			},
			wantName: "www.example.com", wantType: "A", wantTgt: "1.2.3.4",
		},
		{
			name: "CNAME strips trailing dot",
			record: zoneRecord{
				RecordInfo: tidydns.RecordInfo{Type: tidydns.RecordTypeCNAME, Name: "alias", Destination: "target.example.com.", TTL: 300},
				ZoneName:   "example.com",
			},
			wantName: "alias.example.com", wantType: "CNAME", wantTgt: "target.example.com",
		},
		{
			name: "TXT passes through unquoted",
			record: zoneRecord{
				RecordInfo: tidydns.RecordInfo{Type: tidydns.RecordTypeTXT, Name: "txt", Destination: "v=spf1", TTL: 300},
				ZoneName:   "example.com",
			},
			wantName: "txt.example.com", wantType: "TXT", wantTgt: "v=spf1",
		},
		{
			name: "zone apex uses dot name",
			record: zoneRecord{
				RecordInfo: tidydns.RecordInfo{Type: tidydns.RecordTypeA, Name: ".", Destination: "1.2.3.4"},
				ZoneName:   "example.com",
			},
			wantName: "example.com", wantType: "A", wantTgt: "1.2.3.4",
		},
		{
			name: "unsupported type returns nil",
			record: zoneRecord{
				RecordInfo: tidydns.RecordInfo{Type: 99, Name: "bad", Destination: "x"},
				ZoneName:   "example.com",
			},
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ep := parseTidyRecord(&tt.record)
			if tt.wantNil {
				assert.Nil(t, ep)
				return
			}
			assert.NotNil(t, ep)
			assert.Equal(t, ep.DNSName, tt.wantName)
			assert.Equal(t, ep.RecordType, tt.wantType)
			assert.Equal(t, ep.Targets[0], tt.wantTgt)
		})
	}
}

func TestTidyfyName(t *testing.T) {
	tests := []struct {
		name     string
		fqdn     string
		wantName string
		wantID   int
	}{
		{"simple subdomain", "www.example.com", "www", 1},
		{"zone apex", "example.com", ".", 1},
		{"longest zone match", "app.sub.other.com", "app", 3},
		{"falls back to parent zone", "deep.other.com", "deep", 2},
		{"no match", "unknown.org", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, id := tidyfyName(testZones, tt.fqdn)
			assert.Equal(t, name, tt.wantName)
			assert.Equal(t, id, tt.wantID)
		})
	}
}

func TestTidyNameToFQDN(t *testing.T) {
	tests := []struct {
		name, zone, want string
	}{
		{"www", "example.com", "www.example.com"},
		{".", "example.com", "example.com"},
		{"sub.host", "example.com", "sub.host.example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name+"."+tt.zone, func(t *testing.T) {
			assert.Equal(t, tidyNameToFQDN(tt.name, tt.zone), tt.want)
		})
	}
}

func TestClampTTL(t *testing.T) {
	tests := []struct {
		name string
		in   int
		want int
	}{
		{"zero stays zero", 0, 0},
		{"below minimum clamped", 60, 300},
		{"at minimum unchanged", 300, 300},
		{"above minimum unchanged", 600, 600},
		{"negative unchanged", -1, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, clampTTL(tt.in), tt.want)
		})
	}
}

func TestRecordTypeToString(t *testing.T) {
	tests := []struct {
		in   tidydns.RecordType
		want string
	}{
		{tidydns.RecordTypeA, "A"},
		{tidydns.RecordTypeAPTR, "A"},
		{tidydns.RecordTypeCNAME, "CNAME"},
		{tidydns.RecordTypeTXT, "TXT"},
		{tidydns.RecordTypeSRV, "SRV"},
		{tidydns.RecordTypeMX, ""},
		{tidydns.RecordTypeNS, ""},
		{99, ""},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			assert.Equal(t, recordTypeToString(tt.in), tt.want)
		})
	}
}

func TestStringToRecordType(t *testing.T) {
	tests := []struct {
		in   string
		want tidydns.RecordType
		ok   bool
	}{
		{"A", tidydns.RecordTypeA, true},
		{"CNAME", tidydns.RecordTypeCNAME, true},
		{"TXT", tidydns.RecordTypeTXT, true},
		{"SRV", tidydns.RecordTypeSRV, true},
		{"UNKNOWN", 0, false},
		{"MX", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, ok := stringToRecordType(tt.in)
			assert.Equal(t, got, tt.want)
			assert.Equal(t, ok, tt.ok)
		})
	}
}

func TestIsSupportedRecordType(t *testing.T) {
	assert.True(t, isSupportedRecordType("A"))
	assert.True(t, isSupportedRecordType("CNAME"))
	assert.True(t, isSupportedRecordType("TXT"))
	assert.True(t, isSupportedRecordType("SRV"))
	assert.False(t, isSupportedRecordType("MX"))
	assert.False(t, isSupportedRecordType("NS"))
	assert.False(t, isSupportedRecordType("AAAA"))
	assert.False(t, isSupportedRecordType(""))
}

func TestTidyfyTarget(t *testing.T) {
	assert.Equal(t, tidyfyTarget("\"hello\""), "hello")
	assert.Equal(t, tidyfyTarget("noquotes"), "noquotes")
	assert.Equal(t, tidyfyTarget(""), "")
}

func TestRecordsListRecordsError(t *testing.T) {
	client := &mockClient{}
	client.On("ListRecords", mock.Anything, 1).Return(([]*tidydns.RecordInfo)(nil), fmt.Errorf("connection refused"))

	p := newTestProvider(t, client, []*tidydns.ZoneInfo{{ID: 1, Name: "example.com"}})

	eps, err := p.Records(context.Background())
	assert.Error(t, err)
	assert.Nil(t, eps)
}

func TestApplyChangesCreateError(t *testing.T) {
	client := &mockClient{}
	client.On("CreateRecord", mock.Anything, 1, mock.Anything).Return(0, fmt.Errorf("server error"))

	p := newTestProvider(t, client, []*tidydns.ZoneInfo{{ID: 1, Name: "example.com"}})

	changes := &plan.Changes{
		Create: []*endpoint.Endpoint{
			{DNSName: "new.example.com", Targets: endpoint.Targets{"1.1.1.1"}, RecordType: "A"},
		},
	}
	err := p.ApplyChanges(context.Background(), changes)
	assert.Error(t, err)
}

func TestApplyChangesDeleteError(t *testing.T) {
	existing := []*tidydns.RecordInfo{
		{ID: 10, Type: tidydns.RecordTypeA, Name: "old", Destination: "1.1.1.1"},
	}

	client := &mockClient{}
	client.On("FindRecord", mock.Anything, 1, "old", tidydns.RecordTypeA).Return(existing, nil)
	client.On("DeleteRecord", mock.Anything, 1, 10).Return(fmt.Errorf("delete failed"))

	p := newTestProvider(t, client, []*tidydns.ZoneInfo{{ID: 1, Name: "example.com"}})

	changes := &plan.Changes{
		Delete: []*endpoint.Endpoint{
			{DNSName: "old.example.com", Targets: endpoint.Targets{"1.1.1.1"}, RecordType: "A"},
		},
	}
	err := p.ApplyChanges(context.Background(), changes)
	assert.Error(t, err)
}

func TestApplyChangesUpdateMultiTarget(t *testing.T) {
	existing := []*tidydns.RecordInfo{
		{ID: 1, Type: tidydns.RecordTypeA, Name: "multi", Destination: "1.0.0.1"},
		{ID: 2, Type: tidydns.RecordTypeA, Name: "multi", Destination: "1.0.0.2"},
	}

	client := &mockClient{}
	client.On("FindRecord", mock.Anything, 1, "multi", tidydns.RecordTypeA).Return(existing, nil)

	// Two old targets updated to two new targets
	client.On("UpdateRecord", mock.Anything, 1, 1, tidydns.RecordInfo{
		Type: tidydns.RecordTypeA, Name: "multi", Destination: "2.0.0.1", TTL: 300,
	}).Return(nil)
	client.On("UpdateRecord", mock.Anything, 1, 2, tidydns.RecordInfo{
		Type: tidydns.RecordTypeA, Name: "multi", Destination: "2.0.0.2", TTL: 300,
	}).Return(nil)

	p := newTestProvider(t, client, []*tidydns.ZoneInfo{{ID: 1, Name: "example.com"}})

	changes := &plan.Changes{
		UpdateOld: []*endpoint.Endpoint{
			{DNSName: "multi.example.com", Targets: endpoint.Targets{"1.0.0.1", "1.0.0.2"}, RecordType: "A"},
		},
		UpdateNew: []*endpoint.Endpoint{
			{DNSName: "multi.example.com", Targets: endpoint.Targets{"2.0.0.1", "2.0.0.2"}, RecordType: "A", RecordTTL: 300},
		},
	}
	err := p.ApplyChanges(context.Background(), changes)
	assert.NoError(t, err)
	client.AssertExpectations(t)
}

func TestApplyChangesUpdateDeletesExtraOld(t *testing.T) {
	// 2 old targets, only 1 new target => the extra old record is deleted
	existing := []*tidydns.RecordInfo{
		{ID: 1, Type: tidydns.RecordTypeA, Name: "shrink", Destination: "1.0.0.1"},
		{ID: 2, Type: tidydns.RecordTypeA, Name: "shrink", Destination: "1.0.0.2"},
	}

	client := &mockClient{}
	client.On("FindRecord", mock.Anything, 1, "shrink", tidydns.RecordTypeA).Return(existing, nil)

	client.On("UpdateRecord", mock.Anything, 1, 1, tidydns.RecordInfo{
		Type: tidydns.RecordTypeA, Name: "shrink", Destination: "2.0.0.1", TTL: 300,
	}).Return(nil)
	client.On("DeleteRecord", mock.Anything, 1, 2).Return(nil)

	p := newTestProvider(t, client, []*tidydns.ZoneInfo{{ID: 1, Name: "example.com"}})

	changes := &plan.Changes{
		UpdateOld: []*endpoint.Endpoint{
			{DNSName: "shrink.example.com", Targets: endpoint.Targets{"1.0.0.1", "1.0.0.2"}, RecordType: "A"},
		},
		UpdateNew: []*endpoint.Endpoint{
			{DNSName: "shrink.example.com", Targets: endpoint.Targets{"2.0.0.1"}, RecordType: "A", RecordTTL: 300},
		},
	}
	err := p.ApplyChanges(context.Background(), changes)
	assert.NoError(t, err)
	client.AssertExpectations(t)
}

func TestApplyChangesUpdateCreatesExtraNew(t *testing.T) {
	// 1 old target, 2 new targets => an extra record is created
	existing := []*tidydns.RecordInfo{
		{ID: 1, Type: tidydns.RecordTypeA, Name: "grow", Destination: "1.0.0.1"},
	}

	client := &mockClient{}
	client.On("FindRecord", mock.Anything, 1, "grow", tidydns.RecordTypeA).Return(existing, nil)

	client.On("UpdateRecord", mock.Anything, 1, 1, tidydns.RecordInfo{
		Type: tidydns.RecordTypeA, Name: "grow", Destination: "2.0.0.1", TTL: 300,
	}).Return(nil)
	client.On("CreateRecord", mock.Anything, 1, tidydns.RecordInfo{
		Type: tidydns.RecordTypeA, Name: "grow", Destination: "2.0.0.2", TTL: 300,
	}).Return(99, nil)

	p := newTestProvider(t, client, []*tidydns.ZoneInfo{{ID: 1, Name: "example.com"}})

	changes := &plan.Changes{
		UpdateOld: []*endpoint.Endpoint{
			{DNSName: "grow.example.com", Targets: endpoint.Targets{"1.0.0.1"}, RecordType: "A"},
		},
		UpdateNew: []*endpoint.Endpoint{
			{DNSName: "grow.example.com", Targets: endpoint.Targets{"2.0.0.1", "2.0.0.2"}, RecordType: "A", RecordTTL: 300},
		},
	}
	err := p.ApplyChanges(context.Background(), changes)
	assert.NoError(t, err)
	client.AssertExpectations(t)
}

func TestApplyChangesPhaseOrdering(t *testing.T) {
	// Verify deletes complete before creates by setting up a scenario where
	// we delete and create the same record name. If deletes didn't run first
	// the create would succeed but the subsequent delete would remove it.
	deleteRecords := []*tidydns.RecordInfo{
		{ID: 10, Type: tidydns.RecordTypeA, Name: "swap", Destination: "1.0.0.1"},
	}

	client := &mockClient{}
	client.On("FindRecord", mock.Anything, 1, "swap", tidydns.RecordTypeA).Return(deleteRecords, nil)
	client.On("DeleteRecord", mock.Anything, 1, 10).Return(nil)
	client.On("CreateRecord", mock.Anything, 1, tidydns.RecordInfo{
		Type: tidydns.RecordTypeA, Name: "swap", Destination: "2.0.0.2",
	}).Return(20, nil)

	p := newTestProvider(t, client, []*tidydns.ZoneInfo{{ID: 1, Name: "example.com"}})

	changes := &plan.Changes{
		Delete: []*endpoint.Endpoint{
			{DNSName: "swap.example.com", Targets: endpoint.Targets{"1.0.0.1"}, RecordType: "A"},
		},
		Create: []*endpoint.Endpoint{
			{DNSName: "swap.example.com", Targets: endpoint.Targets{"2.0.0.2"}, RecordType: "A"},
		},
	}
	err := p.ApplyChanges(context.Background(), changes)
	assert.NoError(t, err)
	client.AssertExpectations(t)
}

func TestRecordLocation(t *testing.T) {
	tests := []struct {
		name       string
		recordType string
		target     string
		want       int
	}{
		{"heritage TXT gets location 1", RecordTypeTXT, "heritage=external-dns,external-dns/owner=default", 1},
		{"non-heritage TXT gets location 0", RecordTypeTXT, "v=spf1 include:example.com", 0},
		{"A record with non-private IP gets location 0", RecordTypeA, "8.8.8.8", 0},
		{"A record with 10.x private IP gets location 1", RecordTypeA, "10.0.1.5", 1},
		{"A record with 172.16.x private IP gets location 1", RecordTypeA, "172.16.0.1", 1},
		{"A record with 172.31.x private IP gets location 1", RecordTypeA, "172.31.255.255", 1},
		{"A record with 172.32.x gets location 0", RecordTypeA, "172.32.0.1", 0},
		{"A record with 192.168.x private IP gets location 1", RecordTypeA, "192.168.1.100", 1},
		{"CNAME gets location 0", RecordTypeCNAME, "target.example.com.", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, int(recordLocation(tt.recordType, tt.target)), tt.want)
		})
	}
}
