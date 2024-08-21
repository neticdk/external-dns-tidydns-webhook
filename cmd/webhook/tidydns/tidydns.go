package tidydns

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type TidyDNSClient interface {
	ListZones() ([]Zone, error)
	CreateRecord(zoneID int, info *Record) error
	ListRecords(zoneID int) ([]Record, error)
	DeleteRecord(zoneID int, recordID int) error
}

type Record struct {
	ID          int    `json:"id"`
	Type        string `json:"type_name"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Destination string `json:"destination"`
	TTL         int    `json:"ttl"`
	ZoneName    string `json:"zone_name"`
	ZoneID      int    `json:"zone_id"`
}

type Zone struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type tidyDNSClient struct {
	client   *http.Client
	username string
	password string
	baseURL  string
}

type RecordType int

const (
	RecordTypeA     RecordType = 0
	RecordTypeAPTR  RecordType = 1
	RecordTypeCNAME RecordType = 2
	RecordTypeMX    RecordType = 3
	RecordTypeNS    RecordType = 4
	RecordTypeTXT   RecordType = 5
	RecordTypeSRV   RecordType = 6
	RecordTypeDS    RecordType = 7
	RecordTypeSSHFP RecordType = 8
	RecordTypeTLSA  RecordType = 9
	RecordTypeCAA   RecordType = 10
)

func NewTidyDnsClient(baseURL, username, password string, timeout time.Duration) TidyDNSClient {
	slog.Debug("baseURL set to: " + baseURL + " with " + timeout.String() + " timeout")
	return &tidyDNSClient{
		baseURL:  baseURL,
		username: username,
		password: password,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *tidyDNSClient) ListZones() ([]Zone, error) {
	zones := []Zone{}
	err := c.request("GET", "/=/zone?type=json", nil, &zones)
	return zones, err
}

func (c *tidyDNSClient) CreateRecord(zoneID int, info *Record) error {
	recordType, err := encodeRecordType(info.Type)
	if err != nil {
		return err
	}

	data := url.Values{
		"type":        {strconv.Itoa(int(recordType))},
		"name":        {info.Name},
		"ttl":         {strconv.Itoa(info.TTL)},
		"description": {info.Description},
		"status":      {strconv.Itoa(0)},
		"destination": {info.Destination},
		"location_id": {strconv.Itoa(0)},
	}

	url := fmt.Sprintf("/=/record/new/%d", zoneID)
	return c.request("POST", url, strings.NewReader(data.Encode()), nil)
}

func (c *tidyDNSClient) ListRecords(zoneID int) ([]Record, error) {
	records := []Record{}
	url := fmt.Sprintf("/=/record_merged?type=json&zone_id=%d&showall=1", zoneID)
	err := c.request("GET", url, nil, &records)
	return records, err
}

func (c *tidyDNSClient) DeleteRecord(zoneID int, recordID int) error {
	url := fmt.Sprintf("/=/record/%d/%d", recordID, zoneID)
	return c.request("DELETE", url, nil, nil)
}

func (c *tidyDNSClient) request(method, url string, value io.Reader, resp any) error {
	req, err := http.NewRequest(method, (c.baseURL + url), value)
	if err != nil {
		return err
	}

	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	res, err := c.client.Do(req)
	if err != nil {
		return err
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("error from tidyDNS server: %s", res.Status)
	}

	if resp == nil {
		return nil
	} else {
		return json.NewDecoder(res.Body).Decode(resp)
	}
}

// Convert the DNS type represented by a string into a Tidy type-number
func encodeRecordType(t string) (RecordType, error) {
	switch t {
	case "AAAA":
		return RecordTypeA, nil
	case "A":
		return RecordTypeA, nil
	case "CNAME":
		return RecordTypeCNAME, nil
	case "TXT":
		return RecordTypeTXT, nil
	default:
		return RecordType(0), fmt.Errorf("unmapped record type %s", t)
	}
}
