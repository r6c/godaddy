# Godaddy for `libdns`

[![godoc reference](https://img.shields.io/badge/godoc-reference-blue.svg)](https://pkg.go.dev/github.com/libdns/godaddy)

This package implements the libdns interfaces for the [Godaddy API](https://developer.godaddy.com/doc/endpoint/domains), compatible with libdns v1.1.0+

## Configuration

The provider supports the following configuration options:

```go
provider := godaddy.Provider{
    APIToken: "your-api-key:your-api-secret",
    UseOTE:   false,  // true for testing environment, false for production (default)
    HTTPTimeout: 30 * time.Second,  // optional, defaults to 30 seconds
}
```

### Environment Configuration

Based on the [GoDaddy API documentation](https://developer.godaddy.com/doc/endpoint/domains), this provider supports both environments:

- **Production** (default): `https://api.godaddy.com`
- **OTE (Operational Test Environment)**: `https://api.ote-godaddy.com`

Set `UseOTE: true` to use the testing environment during development.

## Example

Here's a minimal example of how to get all your DNS records using this `libdns` provider (see `_example/main.go`)

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	godaddy "github.com/libdns/godaddy"
	"github.com/libdns/libdns"
)

func main() {
	token := os.Getenv("GODADDY_TOKEN")
	if token == "" {
		fmt.Printf("GODADDY_TOKEN not set\n")
		return
	}
	zone := os.Getenv("ZONE")
	if zone == "" {
		fmt.Printf("ZONE not set\n")
		return
	}
	
	provider := godaddy.Provider{
		APIToken:    token,
		UseOTE:      false, // Set to true for testing environment
		HTTPTimeout: 30 * time.Second,
	}

	records, err := provider.GetRecords(context.TODO(), zone)
	if err != nil {
		log.Fatalf("ERROR: %s\n", err.Error())
	}

	testName := "libdns-test"
	hasTestName := false

	for _, record := range records {
		rr := record.RR() // Convert to RR to access common fields
		fmt.Printf("%s (.%s): %s, %s\n", rr.Name, zone, rr.Data, rr.Type)
		if rr.Name == testName {
			hasTestName = true
		}
	}

	if !hasTestName {
		// Create a TXT record using the new typed interface
		appendedRecords, err := provider.AppendRecords(context.TODO(), zone, []libdns.Record{
			libdns.TXT{
				Name: testName,
				TTL:  time.Duration(600) * time.Second,
				Text: "libdns-test-value",
			},
		})

		if err != nil {
			log.Fatalf("ERROR: %s\n", err.Error())
		}

		fmt.Println("appendedRecords")
		for _, record := range appendedRecords {
			rr := record.RR()
			fmt.Printf("  %s: %s (%s)\n", rr.Name, rr.Data, rr.Type)
		}
	} else {
		// Delete the TXT record
		deleteRecords, err := provider.DeleteRecords(context.TODO(), zone, []libdns.Record{
			libdns.TXT{
				Name: testName,
			},
		})

		if err != nil {
			log.Fatalf("ERROR: %s\n", err.Error())
		}

		fmt.Println("deleteRecords")
		for _, record := range deleteRecords {
			rr := record.RR()
			fmt.Printf("  %s: %s (%s)\n", rr.Name, rr.Data, rr.Type)
		}
	}
}

## Supported Record Types

This provider supports the following DNS record types:

- **A/AAAA**: IPv4/IPv6 address records (returned as `libdns.Address`)
- **TXT**: Text records (returned as `libdns.TXT`)
- **CNAME**: Canonical name records (returned as `libdns.CNAME`)
- **MX**: Mail exchange records (returned as `libdns.MX`)
- **NS**: Name server records (returned as `libdns.NS`)
- **Other types**: Unsupported record types are returned as `libdns.RR`

## GoDaddy API Requirements

- **API Token format**: "key:secret" (sso-key format)
- **Minimum TTL**: 600 seconds (automatically enforced)
- **Environments**: 
  - Production: `https://api.godaddy.com`
  - Testing (OTE): `https://api.ote-godaddy.com`
- **Rate Limits**: Follow GoDaddy's API rate limiting guidelines
- **User-Agent**: Automatically set to `libdns-godaddy/1.0`

## Development and Testing

For development and testing, you should:

1. **Use the OTE environment**: Set `UseOTE: true` in your provider configuration
2. **Generate OTE API keys**: Create separate API keys for the OTE environment
3. **Test thoroughly**: Verify all operations in OTE before using in production

Example for testing:
```go
provider := godaddy.Provider{
    APIToken: "your-ote-key:your-ote-secret",
    UseOTE:   true,  // Use testing environment
}
```