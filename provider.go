// Package godaddy implements methods for manipulating GoDaddy DNS records.
// based on GoDaddy Domains API https://developer.godaddy.com/doc/endpoint/domains#/v1
package godaddy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/libdns/libdns"
)

// Provider implements libdns interfaces for GoDaddy DNS
type Provider struct {
	APIToken string `json:"api_token,omitempty"`

	// UseOTE enables the use of GoDaddy's OTE (Operational Test Environment)
	// instead of the production environment. This is useful for development and testing.
	// When true, uses https://api.ote-godaddy.com
	// When false (default), uses https://api.godaddy.com
	UseOTE bool `json:"use_ote,omitempty"`

	// HTTPTimeout specifies the timeout for HTTP requests.
	// If zero, a default timeout of 30 seconds is used.
	HTTPTimeout time.Duration `json:"http_timeout,omitempty"`
}

func getDomain(zone string) string {
	return strings.TrimSuffix(zone, ".")
}

func getRecordName(zone, name string) string {
	if name == "@" {
		return "@"
	}
	return strings.TrimSuffix(strings.TrimSuffix(name, zone), ".")
}

func (p *Provider) getApiHost() string {
	if p.UseOTE {
		return "https://api.ote-godaddy.com"
	}
	return "https://api.godaddy.com"
}

func (p *Provider) getHTTPClient() *http.Client {
	timeout := p.HTTPTimeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &http.Client{
		Timeout: timeout,
	}
}

func (p *Provider) setCommonHeaders(req *http.Request) {
	req.Header.Set("Authorization", "sso-key "+p.APIToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "libdns-godaddy/1.0")
}

// godaddyRecord represents a DNS record as returned by GoDaddy API
type godaddyRecord struct {
	Type string `json:"type"`
	Name string `json:"name"`
	Data string `json:"data"`
	TTL  int    `json:"ttl"`
}

// convertToLibdnsRecord converts a GoDaddy API record to a libdns Record
func convertToLibdnsRecord(gr godaddyRecord) libdns.Record {
	ttl := time.Duration(gr.TTL) * time.Second

	switch strings.ToUpper(gr.Type) {
	case "A", "AAAA":
		ip, err := netip.ParseAddr(gr.Data)
		if err != nil {
			// Fallback to RR if IP parsing fails
			return libdns.RR{
				Name: gr.Name,
				TTL:  ttl,
				Type: gr.Type,
				Data: gr.Data,
			}
		}
		return libdns.Address{
			Name: gr.Name,
			TTL:  ttl,
			IP:   ip,
		}
	case "TXT":
		return libdns.TXT{
			Name: gr.Name,
			TTL:  ttl,
			Text: gr.Data,
		}
	case "CNAME":
		return libdns.CNAME{
			Name:   gr.Name,
			TTL:    ttl,
			Target: gr.Data,
		}
	case "MX":
		// MX data format is "priority target" (e.g., "10 mail.example.com")
		parts := strings.SplitN(gr.Data, " ", 2)
		var preference uint16
		var target string
		if len(parts) == 2 {
			if pref, err := strconv.ParseUint(parts[0], 10, 16); err == nil {
				preference = uint16(pref)
				target = parts[1]
			} else {
				// If parsing fails, fallback to RR
				return libdns.RR{
					Name: gr.Name,
					TTL:  ttl,
					Type: gr.Type,
					Data: gr.Data,
				}
			}
		} else {
			// Invalid format, fallback to RR
			return libdns.RR{
				Name: gr.Name,
				TTL:  ttl,
				Type: gr.Type,
				Data: gr.Data,
			}
		}
		return libdns.MX{
			Name:       gr.Name,
			TTL:        ttl,
			Preference: preference,
			Target:     target,
		}
	case "NS":
		return libdns.NS{
			Name:   gr.Name,
			TTL:    ttl,
			Target: gr.Data,
		}
	case "SRV":
		// SRV records are complex, using RR as fallback for now
		fallthrough
	default:
		return libdns.RR{
			Name: gr.Name,
			TTL:  ttl,
			Type: gr.Type,
			Data: gr.Data,
		}
	}
}

// GetRecords lists all the records in the zone.
func (p *Provider) GetRecords(ctx context.Context, zone string) ([]libdns.Record, error) {
	client := p.getHTTPClient()
	domain := getDomain(zone)
	var records []libdns.Record

	// Get all DNS records for the domain (most domains don't have enough records to require pagination)
	url := fmt.Sprintf("%s/v1/domains/%s/records", p.getApiHost(), domain)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	p.setCommonHeaders(req)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body for error handling
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed: status %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	var resultObj []godaddyRecord
	if err := json.Unmarshal(bodyBytes, &resultObj); err != nil {
		return nil, fmt.Errorf("failed to parse response JSON: %w", err)
	}

	// convert all records to libdns format
	for _, record := range resultObj {
		records = append(records, convertToLibdnsRecord(record))
	}

	return records, nil
}

// convertFromLibdnsRecord converts a libdns Record to GoDaddy API format
func convertFromLibdnsRecord(record libdns.Record, zone string) (godaddyRecord, error) {
	rr := record.RR()

	// Ensure minimum TTL of 600 seconds as required by GoDaddy
	ttlSeconds := int(rr.TTL / time.Second)
	if ttlSeconds < 600 {
		ttlSeconds = 600
	}

	return godaddyRecord{
		Type: rr.Type,
		Name: getRecordName(zone, rr.Name),
		Data: rr.Data,
		TTL:  ttlSeconds,
	}, nil
}

// AppendRecords adds records to the zone. It returns the records that were added.
func (p *Provider) AppendRecords(ctx context.Context, zone string, records []libdns.Record) ([]libdns.Record, error) {
	var appendedRecords []libdns.Record
	client := p.getHTTPClient()

	for _, record := range records {
		gr, err := convertFromLibdnsRecord(record, zone)
		if err != nil {
			return nil, fmt.Errorf("failed to convert record: %w", err)
		}

		data, err := json.Marshal([]godaddyRecord{gr})
		if err != nil {
			return nil, fmt.Errorf("failed to marshal record data: %w", err)
		}

		url := fmt.Sprintf("%s/v1/domains/%s/records/%s/%s",
			p.getApiHost(), getDomain(zone), gr.Type, gr.Name)

		req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewBuffer(data))
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}
		p.setCommonHeaders(req)
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to execute request: %w", err)
		}

		// Read response for better error handling
		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("failed to append record %s.%s: status %d, body: %s",
				gr.Name, getDomain(zone), resp.StatusCode, string(bodyBytes))
		}

		appendedRecords = append(appendedRecords, record)
	}

	return appendedRecords, nil
}

// SetRecords sets the records in the zone, either by updating existing records
// or creating new ones. It returns the updated records.
func (p *Provider) SetRecords(ctx context.Context, zone string, records []libdns.Record) ([]libdns.Record, error) {
	return p.AppendRecords(ctx, zone, records)
}

// DeleteRecords deletes the records from the zone.
func (p *Provider) DeleteRecords(ctx context.Context, zone string, records []libdns.Record) ([]libdns.Record, error) {
	currentRecords, err := p.GetRecords(ctx, zone)
	if err != nil {
		return nil, fmt.Errorf("failed to get current records: %w", err)
	}

	var deletedRecords []libdns.Record
	client := p.getHTTPClient()

	// Find records that actually exist in the zone
	for _, record := range records {
		recordRR := record.RR()
		recordName := getRecordName(zone, recordRR.Name)

		for _, current := range currentRecords {
			currentRR := current.RR()
			if currentRR.Type == recordRR.Type &&
				getRecordName(zone, currentRR.Name) == recordName {
				deletedRecords = append(deletedRecords, current)
				break
			}
		}
	}

	// Delete verified records with individual API calls
	for _, record := range deletedRecords {
		rr := record.RR()
		recordName := getRecordName(zone, rr.Name)

		url := fmt.Sprintf("%s/v1/domains/%s/records/%s/%s",
			p.getApiHost(), getDomain(zone), rr.Type, recordName)

		req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create delete request: %w", err)
		}
		p.setCommonHeaders(req)

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to execute delete request: %w", err)
		}

		// Read response for better error handling
		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusNoContent {
			return nil, fmt.Errorf("failed to delete record %s.%s: status %d, body: %s",
				recordName, getDomain(zone), resp.StatusCode, string(bodyBytes))
		}
	}

	return deletedRecords, nil
}

// Interface guards
var (
	_ libdns.RecordGetter   = (*Provider)(nil)
	_ libdns.RecordAppender = (*Provider)(nil)
	_ libdns.RecordSetter   = (*Provider)(nil)
	_ libdns.RecordDeleter  = (*Provider)(nil)
)
