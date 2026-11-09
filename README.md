# gojev

Go SDK for [TypeSafe AI](https://typesafe.ai).

## Quickstart

Add the SDK to your project:

```bash
go get github.com/wawan93/gojev
```

Set `TYPESAFE_API_KEY` in your environment, then instantiate and use the client:

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	gojev "github.com/wawan93/gojev"
)

func main() {
	client, err := gojev.NewClient(os.Getenv("TYPESAFE_API_KEY"))
	if err != nil {
		log.Fatal(err)
	}

	resp, err := client.NewSystemOne("I was charged twice. Please fix this ASAP.").
		Choice("category", "What is this ticket about?", map[string]any{
			"billing":   "Payments, invoicing, refunds",
			"technical": "Bugs, outages, integrations",
			"other":     "Anything else",
		}).
		Noul("urgent", "Is the user asking for immediate help?").
		Do(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	if category := resp.Choice("category"); category != nil {
		fmt.Printf("Category: %s (confidence: %.2f)\n", category.Choice, category.Confidence)
	}
	if urgent := resp.Noul("urgent"); urgent != nil {
		fmt.Printf("Urgent probability: %.2f\n", urgent.Noul)
	}

	// Or using a direct SystemOneRequest struct:
	resp, err = client.SystemOne(context.Background(), &gojev.SystemOneRequest{
		State: "I was charged twice. Please fix this ASAP.",
		Questions: map[string]gojev.Question{
			"category": gojev.Choice("What is this ticket about?", map[string]any{
				"billing":   "Payments, invoicing, refunds",
				"technical": "Bugs, outages, integrations",
			}),
			"urgent": gojev.Noul("Is the user asking for immediate help?"),
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}
```

## Images and box questions

Box questions and image assets need Jev 1.14 and the `TypeSafe-Danger-Zone-Images` header:

```go
png, _ := os.ReadFile("screenshot.png")

resp, err := client.NewSystemOne("Screenshot: [screen]").
	Asset("[screen]", gojev.Image("png", png)).
	Box("button", "Locate the submit button.").
	DangerZoneImages().
	Do(ctx)
if err != nil {
	log.Fatal(err)
}
if b := resp.Box("button"); b != nil {
	fmt.Printf("top=%.2f left=%.2f\n", b.Box.Top, b.Box.Left)
}
```

## Retries and timeouts

By default the client retries 408, 429 and every 5xx (including 529, "overloaded") up to 2 times with jittered exponential backoff, honoring `retry-after-ms` and `Retry-After`. The whole call, including retries and delays, is limited to 2 minutes, while each HTTP attempt times out after 30 seconds; change the budget with `RetryPolicy.Timeout`:

```go
policy := gojev.DefaultRetryPolicy()
policy.Timeout = 5 * time.Minute
client, err := gojev.NewClient(apiKey, gojev.WithRetryPolicy(policy))
```

Learn what TypeSafe is, what it can do, and how to use it in [TypeSafe docs](https://docs.typesafe.ai/).
