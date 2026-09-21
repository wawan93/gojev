package gojev_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	gojev "github.com/wawan93/gojev"
)

func TestSystemOneRequestBuilder(t *testing.T) {
	t.Run("build request without client", func(t *testing.T) {
		builder := gojev.NewSystemOneBuilder("Help! My payouts have been failing for 3 days.").
			Noul("is_urgent", "Does this convey urgency?", &gojev.NoulCriteria{
				True:  "Explicitly time-sensitive",
				False: "No urgency expressed",
			}).
			Choice("department", "Which team should handle this?", map[string]any{
				"billing":   "Payments, invoicing, refunds",
				"technical": "Bugs, outages, integrations",
			}).
			Score("frustration", "How frustrated is the customer?", []any{
				"Calm", "Frustrated", "Very angry",
			}).
			Model("custom-model").
			Timeout(5*time.Second).
			Header("X-Custom", "val").
			ExtraBody("custom_param", true)

		req := builder.Build()
		opts := builder.Options()

		if req.State != "Help! My payouts have been failing for 3 days." {
			t.Errorf("unexpected state: %v", req.State)
		}
		if req.Model != "custom-model" {
			t.Errorf("unexpected model: %v", req.Model)
		}
		if len(req.Questions) != 3 {
			t.Fatalf("expected 3 questions, got %d", len(req.Questions))
		}
		if len(opts) != 3 {
			t.Fatalf("expected 3 options, got %d", len(opts))
		}

		// Calling Do on standalone builder should fail because client is not set
		b := gojev.NewSystemOneBuilder("state")
		_, err := b.Do(context.Background())
		if !errors.Is(err, gojev.ErrClientNotAttached) {
			t.Errorf("expected ErrClientNotAttached, got %v", err)
		}
	})

	t.Run("execute with mock server", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/systemone" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}
			if r.Header.Get("Authorization") != "Bearer test-key" {
				t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
			}

			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("failed to decode request body: %v", err)
			}
			qs, _ := body["questions"].(map[string]any)
			if len(qs) != 1 {
				t.Errorf("expected 1 question in body, got %d", len(qs))
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"model": "jev-latest",
				"answers": {
					"urgent": {
						"type": "noul",
						"noul": 0.95
					}
				},
				"usage": {
					"input_tokens": 10,
					"output_tokens": 5
				}
			}`))
		}))
		defer ts.Close()

		client, err := gojev.NewClient("test-key", gojev.WithBaseURL(ts.URL))
		if err != nil {
			t.Fatalf("failed to create client: %v", err)
		}

		ctx := context.Background()

		// Test client.NewSystemOne(state)...Do(ctx)
		resp, err := client.NewSystemOne("Customer is angry").
			Noul("urgent", "Is this urgent?").
			Do(ctx)
		if err != nil {
			t.Fatalf("NewSystemOne().Do() failed: %v", err)
		}
		if n := resp.Noul("urgent"); n == nil || n.Noul != 0.95 {
			t.Errorf("unexpected noul response: %v", n)
		}

		// Test client.SystemOne(ctx, req)
		req := gojev.NewSystemOneBuilder("Customer is angry").
			Noul("urgent", "Is this urgent?").
			Build()
		resp2, err := client.SystemOne(ctx, req)
		if err != nil {
			t.Fatalf("client.SystemOne() failed: %v", err)
		}
		if n := resp2.Noul("urgent"); n == nil || n.Noul != 0.95 {
			t.Errorf("unexpected noul response: %v", n)
		}

		// Test direct SystemOneRequest with gojev.Noul constructor
		resp3, err := client.SystemOne(ctx, &gojev.SystemOneRequest{
			State: "Customer is angry",
			Questions: map[string]gojev.Question{
				"urgent": gojev.Noul("Is this urgent?"),
			},
		})
		if err != nil {
			t.Fatalf("direct SystemOne failed: %v", err)
		}
		if n := resp3.Noul("urgent"); n == nil || n.Noul != 0.95 {
			t.Errorf("unexpected noul response: %v", n)
		}
	})
}
