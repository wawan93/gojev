package gojev_test

import (
	"context"
	"fmt"
	"log"

	gojev "github.com/wawan93/gojev"
)

func ExampleClient_SystemOne() {
	// Initialize client with API key.
	client, err := gojev.NewClient("your-api-key")
	if err != nil {
		log.Fatalf("failed to create client: %v", err)
	}

	ctx := context.Background()

	// Call System One API using the fluent builder
	resp, err := client.NewSystemOne("I was charged twice. Please fix this ASAP.").
		Choice("category", "What is this ticket about?", map[string]any{
			"billing":   nil,
			"technical": nil,
			"other":     nil,
		}).
		Noul("urgent", "Is the user asking for immediate help?").
		Do(ctx)
	if err != nil {
		log.Fatalf("SystemOne failed: %v", err)
	}

	// Extract answers
	category := resp.Choice("category")
	if category != nil {
		fmt.Printf("Category: %s (confidence: %.2f)\n", category.Choice, category.Confidence)
	}

	urgent := resp.Noul("urgent")
	if urgent != nil {
		fmt.Printf("Urgent probability: %.2f\n", urgent.Noul)
	}
}
