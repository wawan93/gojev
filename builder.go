package gojev

import (
	"context"
	"maps"
	"slices"
	"time"
)

// SystemOneRequestBuilder builds a SystemOneRequest.
type SystemOneRequestBuilder struct {
	client *Client
	req    *SystemOneRequest
	opts   []RequestOption
}

// NewSystemOneBuilder initializes a new SystemOneRequestBuilder with the given state.
func NewSystemOneBuilder(state JSONContent) *SystemOneRequestBuilder {
	return &SystemOneRequestBuilder{
		req: &SystemOneRequest{
			State:     state,
			Questions: make(map[string]Question),
		},
	}
}

// Noul adds a yes/no question. An optional NoulCriteria can be passed to clarify what counts as yes or no.
func (b *SystemOneRequestBuilder) Noul(name string, instructions JSONContent, criteria ...*NoulCriteria) *SystemOneRequestBuilder {
	b.req.Questions[name] = Noul(instructions, criteria...)
	return b
}

// Choice adds a multiple-choice question with named alternatives.
func (b *SystemOneRequestBuilder) Choice(name string, instructions JSONContent, criteria map[string]JSONContent) *SystemOneRequestBuilder {
	b.req.Questions[name] = Choice(instructions, criteria)
	return b
}

// Score adds a scoring question with ordered criteria.
func (b *SystemOneRequestBuilder) Score(name string, instructions JSONContent, criteria []JSONContent) *SystemOneRequestBuilder {
	b.req.Questions[name] = Score(instructions, criteria)
	return b
}

// Box adds a box question. It requires Jev 1.14 and DangerZoneImages.
func (b *SystemOneRequestBuilder) Box(name string, instructions JSONContent) *SystemOneRequestBuilder {
	b.req.Questions[name] = Box(instructions)
	return b
}

// Asset adds an image asset under a placeholder key referenced from the
// state, instructions or criteria. It requires DangerZoneImages.
func (b *SystemOneRequestBuilder) Asset(key string, asset Asset) *SystemOneRequestBuilder {
	if b.req.Assets == nil {
		b.req.Assets = make(map[string]Asset)
	}
	b.req.Assets[key] = asset
	return b
}

// DangerZoneImages opts the request in to image assets and box questions.
func (b *SystemOneRequestBuilder) DangerZoneImages() *SystemOneRequestBuilder {
	b.opts = append(b.opts, WithDangerZoneImages())
	return b
}

// DangerZoneLongContext opts the request in to the long-context limits.
func (b *SystemOneRequestBuilder) DangerZoneLongContext() *SystemOneRequestBuilder {
	b.opts = append(b.opts, WithDangerZoneLongContext())
	return b
}

// Question adds an arbitrary Question instance.
func (b *SystemOneRequestBuilder) Question(name string, q Question) *SystemOneRequestBuilder {
	b.req.Questions[name] = q
	return b
}

// Model overrides the default model for this request.
func (b *SystemOneRequestBuilder) Model(model string) *SystemOneRequestBuilder {
	b.req.Model = model
	return b
}

// Timeout sets a request-specific timeout.
func (b *SystemOneRequestBuilder) Timeout(d time.Duration) *SystemOneRequestBuilder {
	b.opts = append(b.opts, WithRequestTimeout(d))
	return b
}

// Header sets a single extra request header.
func (b *SystemOneRequestBuilder) Header(key, value string) *SystemOneRequestBuilder {
	b.opts = append(b.opts, WithRequestHeader(key, value))
	return b
}

// Headers sets multiple extra request headers.
func (b *SystemOneRequestBuilder) Headers(headers map[string]string) *SystemOneRequestBuilder {
	for k, v := range headers {
		b.opts = append(b.opts, WithRequestHeader(k, v))
	}
	return b
}

// ExtraBody adds custom fields to the request JSON body.
func (b *SystemOneRequestBuilder) ExtraBody(key string, value any) *SystemOneRequestBuilder {
	b.opts = append(b.opts, WithRequestExtraBody(key, value))
	return b
}

// Retry sets a custom retry policy for this request.
func (b *SystemOneRequestBuilder) Retry(policy *RetryPolicy) *SystemOneRequestBuilder {
	b.opts = append(b.opts, WithRequestRetry(policy))
	return b
}

// Build returns a snapshot of the constructed SystemOneRequest. Later builder
// calls do not modify requests that were already built.
func (b *SystemOneRequestBuilder) Build() *SystemOneRequest {
	req := *b.req
	req.Questions = maps.Clone(b.req.Questions)
	req.Assets = maps.Clone(b.req.Assets)
	return &req
}

// Options returns a copy of the request options configured on this builder.
func (b *SystemOneRequestBuilder) Options() []RequestOption {
	return slices.Clone(b.opts)
}

// Do executes the request using the attached client.
func (b *SystemOneRequestBuilder) Do(ctx context.Context) (*SystemOneResponse, error) {
	if b.client == nil {
		return nil, ErrClientNotAttached
	}
	return b.client.SystemOne(ctx, b.req, b.opts...)
}
