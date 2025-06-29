package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"

	godaddy "github.com/libdns/godaddy"
	"github.com/libdns/libdns"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

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

	// Configure the provider with environment and timeout options
	provider := godaddy.Provider{
		APIToken:    token,
		UseOTE:      false, // Set to true for testing environment
		HTTPTimeout: 30 * time.Second,
	}

	// Check if we should use OTE environment from env var
	if os.Getenv("GODADDY_USE_OTE") == "true" {
		provider.UseOTE = true
		fmt.Println("Using GoDaddy OTE (testing) environment")
	} else {
		fmt.Println("Using GoDaddy production environment")
	}

	records, err := provider.GetRecords(context.TODO(), zone)
	if err != nil {
		log.Fatalf("ERROR: %s\n", err.Error())
	}

	testName := "_acme-challenge.home"
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
				Name: testName + "." + zone,
				TTL:  0,
				Text: "20HnRk5p6rZd7TXhiMoVEYSjt5OpetC6mdovlTfJ4As",
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
