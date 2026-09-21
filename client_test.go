package gojev_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	gojev "github.com/wawan93/gojev"
)

func TestNewClient(t *testing.T) {
	t.Run("missing API key returns sentinel error", func(t *testing.T) {
		_, err := gojev.NewClient("")
		if !errors.Is(err, gojev.ErrMissingAPIKey) {
			t.Fatalf("expected ErrMissingAPIKey, got %v", err)
		}

		_, err = gojev.NewClient("   \n\t  ")
		if !errors.Is(err, gojev.ErrMissingAPIKey) {
			t.Fatalf("expected ErrMissingAPIKey for whitespace key, got %v", err)
		}
	})

	t.Run("successful creation with options", func(t *testing.T) {
		customHTTPClient := &http.Client{Timeout: 5 * time.Second}
		client, err := gojev.NewClient(
			"test-key",
			gojev.WithBaseURL("https://custom.api.com/"),
			gojev.WithDefaultModel("custom-model"),
			gojev.WithHTTPClient(customHTTPClient),
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if client.BaseURL() != "https://custom.api.com" {
			t.Errorf("expected BaseURL https://custom.api.com, got %s", client.BaseURL())
		}
		if client.DefaultModel() != "custom-model" {
			t.Errorf("expected DefaultModel custom-model, got %s", client.DefaultModel())
		}
	})
}

func TestListModels(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"models": [
				{"name": "jev-latest", "description": "Latest model", "release_date": "2024-01-01"}
			]
		}`))
	}))
	defer ts.Close()

	client, err := gojev.NewClient("test-key", gojev.WithBaseURL(ts.URL))
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	// Models.List without options
	resp, err := client.Models.List(context.Background())
	if err != nil {
		t.Fatalf("Models.List failed: %v", err)
	}
	if len(resp.Models) != 1 || resp.Models[0].Name != "jev-latest" {
		t.Errorf("unexpected models response: %+v", resp.Models)
	}

	// Models.List with options
	resp, err = client.Models.List(context.Background(), gojev.WithRequestTimeout(5*time.Second))
	if err != nil {
		t.Fatalf("Models.List with options failed: %v", err)
	}
	if len(resp.Models) != 1 {
		t.Errorf("expected 1 model, got %d", len(resp.Models))
	}
}

func TestRetryBehavior(t *testing.T) {
	var attempts int32

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		att := atomic.AddInt32(&attempts, 1)
		if att < 3 {
			// Fail first 2 attempts with 500
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error": "internal error"}`))
			return
		}
		// Succeed on 3rd attempt
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"models": []}`))
	}))
	defer ts.Close()

	policy := &gojev.RetryPolicy{
		MaxRetries:     2,
		BackoffInitial: 0.01,
		BackoffMax:     0.05,
		BackoffJitter:  0,
		HTTPStatuses:   []int{500},
	}

	client, err := gojev.NewClient(
		"test-key",
		gojev.WithBaseURL(ts.URL),
		gojev.WithRetryPolicy(policy),
	)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	_, err = client.Models.List(context.Background())
	if err != nil {
		t.Fatalf("expected retry to succeed, got error: %v", err)
	}

	if atomic.LoadInt32(&attempts) != 3 {
		t.Errorf("expected 3 attempts, got %d", atomic.LoadInt32(&attempts))
	}
}

func TestErrorUnwrappingAndExtraction(t *testing.T) {
	err := gojev.NewAPIError(400, []byte(`{"error":"invalid_request"}`), nil, "")

	var badReq *gojev.BadRequestError
	if !errors.As(err, &badReq) {
		t.Fatalf("expected errors.As to match *BadRequestError, got false")
	}

	var apiErr *gojev.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected errors.As to unwrap to *APIError, got false")
	}

	if apiErr.Message != "invalid_request" {
		t.Errorf("expected message 'invalid_request', got %q", apiErr.Message)
	}
}
