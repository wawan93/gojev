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

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(httpClient *http.Client) ClientOption {
	return func(c *Client) {
		c.httpClient = httpClient
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

// WithRequestTimeout sets a timeout for an individual request.
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
		if errors.Is(reqCtx.Err(), context.DeadlineExceeded) {
			return 0, nil, &TimeoutError{Err: err}
		}
		return 0, nil, &ConnectionError{Err: err}
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, resp.Header, &ConnectionError{Err: err}
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

func (c *Client) doRequest(ctx context.Context, method, path string, body any, respObj any, opts *RequestConfig) error {
	var bodyBytes []byte
	if body != nil {
		if opts != nil && len(opts.ExtraBody) > 0 {
			b, err := json.Marshal(body)
			if err != nil {
				return fmt.Errorf("failed to marshal request body: %w", err)
			}
			var merged map[string]any
			if err := json.Unmarshal(b, &merged); err != nil {
				return fmt.Errorf("failed to unmarshal request body for merging: %w", err)
			}
			for k, v := range opts.ExtraBody {
				merged[k] = v
			}
			bodyBytes, err = json.Marshal(merged)
			if err != nil {
				return fmt.Errorf("failed to marshal merged request body: %w", err)
			}
		} else {
			var err error
			bodyBytes, err = json.Marshal(body)
			if err != nil {
				return fmt.Errorf("failed to marshal request body: %w", err)
			}
		}
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

	for attempt := 0; attempt < maxAttempts; attempt++ {
		statusCode, header, err := c.executeAttempt(ctx, method, url, bodyBytes, respObj, opts)
		if err == nil {
			return nil
		}

		if attempt == maxAttempts-1 {
			return err
		}

		if !shouldRetry(ctx, err, statusCode, policy) {
			return err
		}

		if sleepErr := sleepBackoff(ctx, attempt, policy, header); sleepErr != nil {
			return sleepErr
		}
	}

	return nil
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
	delay := policy.BackoffInitial * math.Pow(2, float64(attempt))
	if delay > policy.BackoffMax {
		delay = policy.BackoffMax
	}

	if policy.RespectRetryAfter && header != nil {
		if ra := header.Get("Retry-After"); ra != "" {
			if sec, err := strconv.ParseFloat(ra, 64); err == nil && sec > 0 {
				delay = sec
			}
		}
	} else if policy.BackoffJitter > 0 {
		jitter := (rand.Float64()*2 - 1) * policy.BackoffJitter * delay
		delay += jitter
		if delay < 0 {
			delay = policy.BackoffInitial
		}
	}

	timer := time.NewTimer(time.Duration(delay * float64(time.Second)))
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
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

	model := req.Model
	if model == "" {
		model = c.defaultModel
	}

	var rc RequestConfig
	for _, opt := range opts {
		opt(&rc)
	}

	reqBody := struct {
		State     JSONContent         `json:"state"`
		Model     string              `json:"model"`
		Questions map[string]Question `json:"questions"`
	}{
		State:     req.State,
		Model:     model,
		Questions: req.Questions,
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
