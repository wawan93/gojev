package gojev_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	gojev "github.com/wawan93/gojev"
)

func TestDefaultRetryPolicyStatuses(t *testing.T) {
	statuses := gojev.DefaultRetryPolicy().HTTPStatuses
	for _, s := range []int{408, 429, 500, 503, 509, 529, 599} {
		if !slices.Contains(statuses, s) {
			t.Errorf("default policy does not retry %d", s)
		}
	}
	for _, s := range []int{400, 401, 404, 422} {
		if slices.Contains(statuses, s) {
			t.Errorf("default policy retries %d", s)
		}
	}
}

func TestAPIErrorDetailParsing(t *testing.T) {
	err := gojev.NewAPIError(400, []byte(`{"detail":"invalid UTF-8"}`), nil, "")
	var bad *gojev.BadRequestError
	if !errors.As(err, &bad) || bad.Message != "invalid UTF-8" {
		t.Errorf("unexpected 400 error: %v", err)
	}

	err = gojev.NewAPIError(422, []byte(`{"detail":[{"loc":["body","state"],"msg":"Field required","type":"missing"}]}`), nil, "")
	var unp *gojev.UnprocessableEntityError
	if !errors.As(err, &unp) {
		t.Fatalf("expected *UnprocessableEntityError, got %T", err)
	}
	if unp.Message != "body.state: Field required" || len(unp.Errors) != 1 || unp.Errors[0].Type != "missing" {
		t.Errorf("unexpected 422 parsing: %q %+v", unp.Message, unp.Errors)
	}

	var tooLarge *gojev.RequestTooLargeError
	if !errors.As(gojev.NewAPIError(413, nil, nil, ""), &tooLarge) {
		t.Error("413 not mapped to *RequestTooLargeError")
	}
	var media *gojev.UnsupportedMediaTypeError
	if !errors.As(gojev.NewAPIError(415, nil, nil, ""), &media) {
		t.Error("415 not mapped to *UnsupportedMediaTypeError")
	}
	var over *gojev.OverloadedError
	if !errors.As(gojev.NewAPIError(529, nil, nil, ""), &over) {
		t.Error("529 not mapped to *OverloadedError")
	}
	var ise *gojev.InternalServerError
	if !errors.As(gojev.NewAPIError(529, nil, nil, ""), &ise) {
		t.Error("529 no longer matches *InternalServerError")
	}
}

func TestOverloadedIsRetriedByDefault(t *testing.T) {
	var attempts int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&attempts, 1) == 1 {
			w.Header().Set("retry-after-ms", "10")
			w.WriteHeader(529)
			return
		}
		_, _ = w.Write([]byte(`{"models": []}`))
	}))
	defer ts.Close()

	client, _ := gojev.NewClient("k", gojev.WithBaseURL(ts.URL))
	if _, err := client.Models.List(context.Background()); err != nil {
		t.Fatalf("expected 529 to be retried, got %v", err)
	}
	if atomic.LoadInt32(&attempts) != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

func TestRetryPolicyTimeoutBoundsWholeCall(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("retry-after-ms", "100")
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer ts.Close()

	policy := gojev.DefaultRetryPolicy()
	policy.MaxRetries = 50
	policy.Timeout = 250 * time.Millisecond
	client, _ := gojev.NewClient("k", gojev.WithBaseURL(ts.URL), gojev.WithRetryPolicy(policy))

	start := time.Now()
	_, err := client.Models.List(context.Background())
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("call ran %v, past its 250ms budget", elapsed)
	}
	var apiErr *gojev.InternalServerError
	if !errors.As(err, &apiErr) {
		t.Errorf("expected the last API error after the budget ran out, got %T: %v", err, err)
	}
}

func TestBoxQuestionWithImageAsset(t *testing.T) {
	var body map[string]any
	var imagesHeader string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		imagesHeader = r.Header.Get(gojev.HeaderDangerZoneImages)
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		_, _ = w.Write([]byte(`{"model":"jev-1.14.0","answers":{"rect":{"type":"box","box":{"top":0.1,"bottom":0.5,"left":0.2,"right":0.6},"confidence":null}},"usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer ts.Close()

	client, _ := gojev.NewClient("k", gojev.WithBaseURL(ts.URL))
	resp, err := client.NewSystemOne("see [img]").
		Box("rect", "Locate the red rectangle.").
		Asset("[img]", gojev.Image("png", []byte{0x89, 'P', 'N', 'G'})).
		DangerZoneImages().
		Do(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if imagesHeader != "true" {
		t.Errorf("expected %s: true, got %q", gojev.HeaderDangerZoneImages, imagesHeader)
	}
	q := body["questions"].(map[string]any)["rect"].(map[string]any)
	if q["type"] != "box" {
		t.Errorf("unexpected box question: %v", q)
	}
	asset := body["assets"].(map[string]any)["[img]"].(map[string]any)
	if asset["type"] != "image" || asset["format"] != "png" || asset["data_base64"] != "iVBORw==" {
		t.Errorf("unexpected asset: %v", asset)
	}

	box := resp.Box("rect")
	if box == nil || box.Box.Top != 0.1 || box.Box.Right != 0.6 || box.Confidence != nil {
		t.Errorf("unexpected box answer: %+v", box)
	}
	out, _ := json.Marshal(resp.Answers["rect"])
	if !strings.Contains(string(out), `"type":"box"`) {
		t.Errorf("box answer did not round-trip: %s", out)
	}
}
