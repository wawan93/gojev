package gojev

import (
	"context"
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

// Build returns the constructed SystemOneRequest.
func (b *SystemOneRequestBuilder) Build() *SystemOneRequest {
	return b.req
}

// Options returns any request options configured on this builder.
func (b *SystemOneRequestBuilder) Options() []RequestOption {
	return b.opts
}

// Do executes the request using the attached client.
func (b *SystemOneRequestBuilder) Do(ctx context.Context) (*SystemOneResponse, error) {
	if b.client == nil {
		return nil, ErrClientNotAttached
	}
	return b.client.SystemOne(ctx, b.req, b.opts...)
}
