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
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	otel "go.opentelemetry.io/otel/metric"
)

type TidyDNSClient interface {
	ListZones() ([]Zone, error)
	CreateRecord(zoneID json.Number, info *Record) error
	ListRecords(zoneID json.Number) ([]Record, error)
	DeleteRecord(zoneID json.Number, recordID json.Number) error
}

type Record struct {
	ID          json.Number `json:"id"`
	Type        string      `json:"type_name"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Destination string      `json:"destination"`
	TTL         json.Number `json:"ttl"`
	ZoneName    string      `json:"zone_name"`
	ZoneID      json.Number `json:"zone_id"`
}

type Zone struct {
	ID   json.Number `json:"id"`
	Name string      `json:"name"`
}

type tidyDNSClient struct {
	client   *http.Client
	username string
	password string
	baseURL  string
	counter  counter
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

func NewTidyDnsClient(baseURL, username, password string, timeout time.Duration, meter otel.Meter) (TidyDNSClient, error) {
	slog.Debug("baseURL set to: " + baseURL + " with " + timeout.String() + " timeout")

	counter, err := counterProvider(meter, "tidy_requests", ("Requtest made to " + baseURL))
	if err != nil {
		return nil, err
	}

	return &tidyDNSClient{
		baseURL:  baseURL,
		username: username,
		password: password,
		client: &http.Client{
			Timeout: timeout,
		},
		counter: counter,
	}, nil
}

func (c *tidyDNSClient) ListZones() ([]Zone, error) {
	zones := []Zone{}
	err := c.request("GET", "/=/zone?type=json", nil, &zones)
	return zones, err
}

func (c *tidyDNSClient) CreateRecord(zoneID json.Number, info *Record) error {
	recordType, err := encodeRecordType(info.Type)
	if err != nil {
		return err
	}

	ttl := info.TTL.String()

	data := url.Values{
		"type":        {strconv.Itoa(int(recordType))},
		"name":        {info.Name},
		"ttl":         {ttl},
		"description": {info.Description},
		"status":      {strconv.Itoa(0)},
		"destination": {info.Destination},
		"location_id": {strconv.Itoa(0)},
	}

	url := fmt.Sprintf("/=/record/new/%s", zoneID)
	return c.request("POST", url, strings.NewReader(data.Encode()), nil)
}

func (c *tidyDNSClient) ListRecords(zoneID json.Number) ([]Record, error) {
	records := []Record{}
	url := fmt.Sprintf("/=/record_merged?type=json&zone_id=%s&showall=1", zoneID)
	err := c.request("GET", url, nil, &records)
	return records, err
}

func (c *tidyDNSClient) DeleteRecord(zoneID json.Number, recordID json.Number) error {
	url := fmt.Sprintf("/=/record/%s/%s", recordID, zoneID)
	return c.request("DELETE", url, nil, nil)
}

func (c *tidyDNSClient) request(method, url string, value io.Reader, resp any) error {
	slog.Debug(method + " " + c.baseURL + url)
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

	// Tidy uses a strange /= prefix after the base address. Remove this first
	urlPath, _ := strings.CutPrefix(url, "/=")
	// Remove all parameters from the URL
	urlPath, _, _ = strings.Cut(urlPath, "?")

	c.counter(method, urlPath, res.StatusCode)

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
