// Package gojev provides a Go client library for TypeSafe AI.
package gojev

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultBaseURL is the default TypeSafe AI API base URL.
	DefaultBaseURL = "https://api.typesafe.ai"
	// DefaultModel is the default model used for evaluation requests.
	DefaultModel = "jev-latest"

	// maxRetryAfter caps how long a server-provided Retry-After may delay a retry.
	maxRetryAfter = 60.0 // seconds

	// HeaderDangerZoneImages must be "true" for requests that use image assets or box questions.
	HeaderDangerZoneImages = "TypeSafe-Danger-Zone-Images"
	// HeaderDangerZoneLongContext set to "true" raises the model's context limits.
	HeaderDangerZoneLongContext = "TypeSafe-Danger-Zone-Long-Context"

	// maxDelaySeconds is the largest delay that fits in a time.Duration.
	maxDelaySeconds = float64(math.MaxInt64 / int64(time.Second))
)

var (
	// ErrMissingAPIKey is returned when the API key is empty or whitespace.
	ErrMissingAPIKey = errors.New("API key is required")

	// ErrNilRequest is returned when SystemOne receives a nil request.
	ErrNilRequest = errors.New("request cannot be nil")

	// ErrEmptyQuestions is returned when the questions map is empty.
	ErrEmptyQuestions = errors.New("questions map cannot be empty")

	// ErrClientNotAttached is returned when Do() is called on a builder without an attached client.
	ErrClientNotAttached = errors.New("client is not attached to this builder; call Build() and pass to client.SystemOne(ctx, req) or initialize with client.NewSystemOne(state)")
)

// Client is the TypeSafe AI API client.
type Client struct {
	httpClient   *http.Client
	baseURL      string
	apiKey       string
	defaultModel string
	retryPolicy  *RetryPolicy

	// Models provides access to the models resource, matching official SDKs.
	Models *ModelsService
}

// ModelsService provides access to models endpoints.
type ModelsService struct {
	client *Client
}

// List retrieves the list of available models.
func (s *ModelsService) List(ctx context.Context, opts ...RequestOption) (*ListModelsResponse, error) {
	return s.client.listModels(ctx, opts...)
}

// BaseURL returns the configured base URL.
func (c *Client) BaseURL() string {
	return c.baseURL
}

// DefaultModel returns the configured default model.
func (c *Client) DefaultModel() string {
	return c.defaultModel
}

// ClientOption allows configuring the client.
type ClientOption func(*Client)

// WithHTTPClient sets a custom HTTP client. A nil client is ignored.
func WithHTTPClient(httpClient *http.Client) ClientOption {
	return func(c *Client) {
		if httpClient != nil {
			c.httpClient = httpClient
		}
	}
}

// WithBaseURL sets a custom base URL.
func WithBaseURL(baseURL string) ClientOption {
	return func(c *Client) {
		c.baseURL = strings.TrimRight(baseURL, "/")
	}
}

// WithDefaultModel sets the default model.
func WithDefaultModel(model string) ClientOption {
	return func(c *Client) {
		c.defaultModel = model
	}
}

// WithRetryPolicy sets the client-level default retry policy.
func WithRetryPolicy(policy *RetryPolicy) ClientOption {
	return func(c *Client) {
		c.retryPolicy = policy
	}
}

// NewClient creates a new TypeSafe AI API client.
func NewClient(apiKey string, opts ...ClientOption) (*Client, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil, ErrMissingAPIKey
	}

	c := &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL:      DefaultBaseURL,
		apiKey:       apiKey,
		defaultModel: DefaultModel,
		retryPolicy:  DefaultRetryPolicy(),
	}

	for _, opt := range opts {
		opt(c)
	}

	c.Models = &ModelsService{client: c}

	return c, nil
}

// RequestConfig holds options for an individual API request.
type RequestConfig struct {
	Retry        *RetryPolicy
	Timeout      time.Duration
	ExtraHeaders map[string]string
	ExtraBody    map[string]any
}

// RequestOption configures individual API requests.
type RequestOption func(*RequestConfig)

// WithRequestTimeout sets a timeout for each individual HTTP attempt. Use
// RetryPolicy.Timeout to limit the whole call including retries.
func WithRequestTimeout(d time.Duration) RequestOption {
	return func(rc *RequestConfig) {
		rc.Timeout = d
	}
}

// WithRequestHeader adds an extra HTTP header to the request.
func WithRequestHeader(key, value string) RequestOption {
	return func(rc *RequestConfig) {
		if rc.ExtraHeaders == nil {
			rc.ExtraHeaders = make(map[string]string)
		}
		rc.ExtraHeaders[key] = value
	}
}

// WithDangerZoneImages opts the request in to image assets and box questions.
func WithDangerZoneImages() RequestOption {
	return WithRequestHeader(HeaderDangerZoneImages, "true")
}

// WithDangerZoneLongContext opts the request in to the long-context limits.
func WithDangerZoneLongContext() RequestOption {
	return WithRequestHeader(HeaderDangerZoneLongContext, "true")
}

// WithRequestRetry sets a custom retry policy for an individual request.
func WithRequestRetry(policy *RetryPolicy) RequestOption {
	return func(rc *RequestConfig) {
		rc.Retry = policy
	}
}

// WithRequestExtraBody adds a custom field to the request JSON body.
func WithRequestExtraBody(key string, value any) RequestOption {
	return func(rc *RequestConfig) {
		if rc.ExtraBody == nil {
			rc.ExtraBody = make(map[string]any)
		}
		rc.ExtraBody[key] = value
	}
}

func (c *Client) executeAttempt(ctx context.Context, method, url string, bodyBytes []byte, respObj any, opts *RequestConfig) (int, http.Header, error) {
	reqCtx := ctx
	if opts != nil && opts.Timeout > 0 {
		var cancel context.CancelFunc
		reqCtx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	var bodyReader io.Reader
	if bodyBytes != nil {
		bodyReader = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequestWithContext(reqCtx, method, url, bodyReader)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	if bodyReader != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if opts != nil {
		for k, v := range opts.ExtraHeaders {
			req.Header.Set(k, v)
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, transportError(reqCtx, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, resp.Header, transportError(reqCtx, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, resp.Header, NewAPIError(resp.StatusCode, respBody, resp.Header, "")
	}

	if respObj != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, respObj); err != nil {
			return resp.StatusCode, resp.Header, fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return resp.StatusCode, resp.Header, nil
}

// transportError classifies a failure to send a request or read its response
// as a TimeoutError (context deadline or net-level timeout, such as
// http.Client.Timeout) or a ConnectionError.
func transportError(ctx context.Context, err error) error {
	var ne net.Error
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout()) {
		return &TimeoutError{Err: err}
	}
	return &ConnectionError{Err: err}
}

// encodeBody marshals body and merges any ExtraBody fields into it. The merge
// keeps the original fields as raw JSON so numbers are not re-encoded through float64.
func encodeBody(body any, opts *RequestConfig) ([]byte, error) {
	if body == nil {
		return nil, nil
	}
	b, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}
	if opts == nil || len(opts.ExtraBody) == 0 {
		return b, nil
	}

	var merged map[string]json.RawMessage
	if err := json.Unmarshal(b, &merged); err != nil {
		return nil, fmt.Errorf("failed to unmarshal request body for merging: %w", err)
	}
	if merged == nil {
		return nil, errors.New("request body must be a JSON object to merge extra body fields")
	}
	for k, v := range opts.ExtraBody {
		raw, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal extra body field %q: %w", k, err)
		}
		merged[k] = raw
	}
	b, err = json.Marshal(merged)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal merged request body: %w", err)
	}
	return b, nil
}

func (c *Client) doRequest(ctx context.Context, method, path string, body any, respObj any, opts *RequestConfig) error {
	bodyBytes, err := encodeBody(body, opts)
	if err != nil {
		return err
	}

	policy := c.retryPolicy
	if opts != nil && opts.Retry != nil {
		policy = opts.Retry
	}

	maxAttempts := 1
	if policy != nil && policy.MaxRetries > 0 {
		maxAttempts = 1 + policy.MaxRetries
	}

	url := c.baseURL + path

	// The policy's Timeout bounds the whole call: every attempt and every delay.
	callCtx := ctx
	if policy != nil && policy.Timeout > 0 {
		var cancel context.CancelFunc
		callCtx, cancel = context.WithTimeout(ctx, policy.Timeout)
		defer cancel()
	}

	for attempt := 0; ; attempt++ {
		statusCode, header, err := c.executeAttempt(callCtx, method, url, bodyBytes, respObj, opts)
		if err == nil {
			return nil
		}

		if attempt >= maxAttempts-1 || !shouldRetry(callCtx, err, statusCode, policy) {
			return err
		}

		if sleepErr := sleepBackoff(callCtx, attempt, policy, header); sleepErr != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			// The call's time budget ran out while waiting: report the last failure.
			return err
		}
	}
}

func shouldRetry(ctx context.Context, err error, statusCode int, policy *RetryPolicy) bool {
	if policy == nil || ctx.Err() != nil {
		return false
	}
	if shouldRetryError(err, policy) {
		return true
	}
	if statusCode > 0 && shouldRetryStatus(statusCode, policy) {
		return true
	}
	return false
}

func shouldRetryError(err error, policy *RetryPolicy) bool {
	var te *TimeoutError
	if errors.As(err, &te) {
		return policy.APITimeoutError
	}
	var ce *ConnectionError
	if errors.As(err, &ce) {
		return policy.APIConnectionError
	}
	return false
}

func shouldRetryStatus(statusCode int, policy *RetryPolicy) bool {
	for _, s := range policy.HTTPStatuses {
		if s == statusCode {
			return true
		}
	}
	return false
}

func sleepBackoff(ctx context.Context, attempt int, policy *RetryPolicy, header http.Header) error {
	wait := time.Duration(backoffDelay(attempt, policy, header) * float64(time.Second))

	// Don't sleep through the rest of the deadline only to fail afterwards.
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < wait {
		return context.DeadlineExceeded
	}

	timer := time.NewTimer(wait)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// backoffDelay returns the delay in seconds before the given retry attempt:
// jittered exponential backoff, overridden by a valid Retry-After header when
// the policy respects it.
func backoffDelay(attempt int, policy *RetryPolicy, header http.Header) float64 {
	if policy.RespectRetryAfter {
		if sec, ok := retryAfterSeconds(header); ok {
			return sec
		}
	}

	delay := policy.BackoffInitial * math.Pow(2, float64(attempt))
	if policy.BackoffJitter > 0 {
		delay += (rand.Float64()*2 - 1) * policy.BackoffJitter * delay
		if delay < 0 {
			delay = policy.BackoffInitial
		}
	}
	if policy.BackoffMax > 0 && delay > policy.BackoffMax {
		// Jitter below the cap instead of clamping, so capped retries stay spread out.
		delay = policy.BackoffMax * (1 - rand.Float64()*math.Min(policy.BackoffJitter, 1))
	}
	// Keep the delay representable as a time.Duration even when uncapped.
	return math.Min(delay, maxDelaySeconds)
}

// retryAfterSeconds reads the retry-after-ms header, or else Retry-After given
// either as seconds or as an HTTP date, capped at maxRetryAfter. It reports
// false when neither header is present, parsable and in the future.
func retryAfterSeconds(header http.Header) (float64, bool) {
	if ms, err := strconv.ParseFloat(header.Get("Retry-After-Ms"), 64); err == nil && ms > 0 {
		return math.Min(ms/1000, maxRetryAfter), true
	}
	ra := header.Get("Retry-After")
	if ra == "" {
		return 0, false
	}
	sec, err := strconv.ParseFloat(ra, 64)
	if err != nil {
		t, perr := http.ParseTime(ra)
		if perr != nil {
			return 0, false
		}
		sec = time.Until(t).Seconds()
	}
	if !(sec > 0) {
		return 0, false
	}
	return math.Min(sec, maxRetryAfter), true
}

// NewSystemOne starts a fluent SystemOne request builder bound to this client.
func (c *Client) NewSystemOne(state JSONContent) *SystemOneRequestBuilder {
	b := NewSystemOneBuilder(state)
	b.client = c
	return b
}

// SystemOne answers named questions about text or structured state, matching official Python & JS SDKs.
func (c *Client) SystemOne(ctx context.Context, req *SystemOneRequest, opts ...RequestOption) (*SystemOneResponse, error) {
	if req == nil {
		return nil, ErrNilRequest
	}
	if len(req.Questions) == 0 {
		return nil, ErrEmptyQuestions
	}

	reqBody := *req
	if reqBody.Model == "" {
		reqBody.Model = c.defaultModel
	}

	var rc RequestConfig
	for _, opt := range opts {
		opt(&rc)
	}

	var res SystemOneResponse
	if err := c.doRequest(ctx, http.MethodPost, "/v1/systemone", reqBody, &res, &rc); err != nil {
		return nil, err
	}

	return &res, nil
}

func (c *Client) listModels(ctx context.Context, opts ...RequestOption) (*ListModelsResponse, error) {
	var rc RequestConfig
	for _, opt := range opts {
		opt(&rc)
	}

	var res ListModelsResponse
	if err := c.doRequest(ctx, http.MethodGet, "/v1/models", nil, &res, &rc); err != nil {
		return nil, err
	}
	return &res, nil
}
