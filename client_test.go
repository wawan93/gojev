package gojev_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestHTTPClientTimeoutIsTimeoutError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(time.Second):
		}
	}))
	defer ts.Close()

	client, err := gojev.NewClient("test-key",
		gojev.WithBaseURL(ts.URL),
		gojev.WithHTTPClient(&http.Client{Timeout: 20 * time.Millisecond}),
		gojev.WithRetryPolicy(nil),
	)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	_, err = client.Models.List(context.Background())
	var te *gojev.TimeoutError
	if !errors.As(err, &te) {
		t.Fatalf("expected *TimeoutError, got %T: %v", err, err)
	}
}

func TestWithHTTPClientNilIsIgnored(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"models": []}`))
	}))
	defer ts.Close()

	client, err := gojev.NewClient("test-key", gojev.WithBaseURL(ts.URL), gojev.WithHTTPClient(nil))
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	if _, err := client.Models.List(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExtraBodyPreservesLargeIntegers(t *testing.T) {
	var got string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = string(b)
		_, _ = w.Write([]byte(`{"answers": {}}`))
	}))
	defer ts.Close()

	client, err := gojev.NewClient("test-key", gojev.WithBaseURL(ts.URL))
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	_, err = client.NewSystemOne(map[string]any{"order_id": int64(9007199254740993)}).
		Noul("q", "?").
		ExtraBody("custom", true).
		Do(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "9007199254740993") || !strings.Contains(got, `"custom":true`) {
		t.Errorf("request body lost precision or extra field: %s", got)
	}
}

func TestAnswerJSONRoundTrip(t *testing.T) {
	in := []byte(`{"model":"m","answers":{"a":{"type":"noul","noul":0.9},"b":{"type":"choice","choice":"x","confidence":0.8,"probabilities":{"x":0.8}}},"usage":{"input_tokens":1,"output_tokens":2}}`)
	var r1 gojev.SystemOneResponse
	if err := json.Unmarshal(in, &r1); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	out, err := json.Marshal(r1)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var r2 gojev.SystemOneResponse
	if err := json.Unmarshal(out, &r2); err != nil {
		t.Fatalf("re-unmarshal %s: %v", out, err)
	}
	if n := r2.Noul("a"); n == nil || n.Noul != 0.9 {
		t.Errorf("noul lost in round trip: %s", out)
	}
	if c := r2.Choice("b"); c == nil || c.Choice != "x" {
		t.Errorf("choice lost in round trip: %s", out)
	}

	// The type field comes from the answer kind, not the inner struct.
	built, err := json.Marshal(gojev.Answer{Noul: &gojev.NoulAnswer{Noul: 0.5}})
	if err != nil || !strings.Contains(string(built), `"type":"noul"`) {
		t.Errorf("expected type noul in %s (err %v)", built, err)
	}

	// Unknown answer types survive a round trip.
	unknown := `{"type":"rank","order":["a","b"]}`
	var ua gojev.Answer
	if err := json.Unmarshal([]byte(unknown), &ua); err != nil {
		t.Fatalf("unmarshal unknown: %v", err)
	}
	if out, _ := json.Marshal(ua); string(out) != unknown {
		t.Errorf("unknown answer not preserved: %s", out)
	}

	// Reusing an Answer must not leave stale pointers from the previous decode.
	var a gojev.Answer
	_ = json.Unmarshal([]byte(`{"type":"noul","noul":1}`), &a)
	_ = json.Unmarshal([]byte(`{"type":"choice","choice":"x"}`), &a)
	if a.Noul != nil {
		t.Errorf("stale Noul pointer after decoding a choice answer")
	}
}

func TestQuestionLiteralsMarshalType(t *testing.T) {
	cases := map[string]gojev.Question{
		`"type":"noul"`:   gojev.NoulQuestion{Instructions: "?"},
		`"type":"choice"`: gojev.ChoiceQuestion{Instructions: "?"},
		`"type":"score"`:  gojev.ScoreQuestion{Instructions: "?"},
	}
	for want, q := range cases {
		b, err := json.Marshal(q)
		if err != nil || !strings.Contains(string(b), want) {
			t.Errorf("expected %s in %s (err %v)", want, b, err)
		}
	}
}
