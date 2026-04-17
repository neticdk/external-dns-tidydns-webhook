package tidydns

import (
	"strings"

	gotidydns "github.com/neticdk/tidydns-go/pkg/tidydns"
)

// DNS record type constants used throughout the provider.
const (
	RecordTypeA     = "A"
	RecordTypeCNAME = "CNAME"
	RecordTypeTXT   = "TXT"
	RecordTypeSRV   = "SRV"
)

func tidyNameToFQDN(name, zone string) string {
	if name == "." {
		return zone
	}
	return name + "." + zone
}

func clampTTL(ttl int) int {
	if ttl > 0 && ttl < 300 {
		return 300
	}
	return ttl
}

// tidyfyName finds the longest matching zone for the given FQDN and returns
// the record name relative to that zone.
func tidyfyName(zones []*gotidydns.ZoneInfo, name string) (string, int) {
	var bestZone *gotidydns.ZoneInfo
	for _, zone := range zones {
		if name != zone.Name && !strings.HasSuffix(name, "."+zone.Name) {
			continue
		}
		if bestZone == nil || len(zone.Name) > len(bestZone.Name) {
			bestZone = zone
		}
	}
	if bestZone == nil {
		return "", 0
	}
	if name == bestZone.Name {
		return ".", bestZone.ID
	}
	trimmed := strings.TrimSuffix(name, "."+bestZone.Name)
	return trimmed, bestZone.ID
}

func tidyfyTarget(target string) string {
	return strings.Trim(target, "\"")
}

func recordTypeToString(rt gotidydns.RecordType) string {
	switch rt {
	case gotidydns.RecordTypeA, gotidydns.RecordTypeAPTR:
		return RecordTypeA
	case gotidydns.RecordTypeCNAME:
		return RecordTypeCNAME
	case gotidydns.RecordTypeTXT:
		return RecordTypeTXT
	case gotidydns.RecordTypeSRV:
		return RecordTypeSRV
	default:
		return ""
	}
}

func stringToRecordType(s string) (gotidydns.RecordType, bool) {
	switch s {
	case RecordTypeA:
		return gotidydns.RecordTypeA, true
	case RecordTypeCNAME:
		return gotidydns.RecordTypeCNAME, true
	case RecordTypeTXT:
		return gotidydns.RecordTypeTXT, true
	case RecordTypeSRV:
		return gotidydns.RecordTypeSRV, true
	default:
		return 0, false
	}
}

func isSupportedRecordType(s string) bool {
	switch s {
	case RecordTypeA, RecordTypeCNAME, RecordTypeTXT, RecordTypeSRV:
		return true
	default:
		return false
	}
}

// registryTXTLocation returns LocationID 1 for TXT records that are
// external-dns registry ownership records (identified by the
// "heritage=external-dns" marker in their target). All other records
// get LocationID 0 (the default).
func registryTXTLocation(recordType string, target string) gotidydns.LocationID {
	if recordType == RecordTypeTXT && strings.Contains(target, "heritage=external-dns") {
		return 1
	}
	return 0
}
