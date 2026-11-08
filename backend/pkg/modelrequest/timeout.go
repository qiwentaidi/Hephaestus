// Package modelrequest applies channel deadlines to complete model responses,
// including streaming bodies, without imposing a deadline on the conversation.
package modelrequest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"hephaestus-backend/pkg/config"
)

type TimeoutError struct{ Duration time.Duration }

func (e *TimeoutError) Error() string {
	return fmt.Sprintf("单次模型请求已达到 %g 分钟超时限制。可在「AI 配置 → 当前渠道 → 高级设置」中调整。", e.Duration.Minutes())
}
func (e *TimeoutError) Unwrap() error   { return context.DeadlineExceeded }
func (e *TimeoutError) Timeout() bool   { return true }
func (e *TimeoutError) Temporary() bool { return true }

// Transport takes a fresh timeout snapshot for every attempt. The timer remains
// active until the body is consumed or closed, and parent cancellation wins.
type Transport struct {
	Base    http.RoundTripper
	Timeout func() time.Duration
}

func ForProvider(base http.RoundTripper, providerID, baseURL, apiKey, model string, fallbackMinutes int) http.RoundTripper {
	return Transport{Base: base, Timeout: func() time.Duration {
		return config.LiveAIRequestTimeout(providerID, baseURL, apiKey, model, fallbackMinutes)
	}}
}

func (t Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	timeout := time.Duration(config.DefaultAIRequestTimeoutMinutes) * time.Minute
	if t.Timeout != nil {
		timeout = t.Timeout()
	}
	ctx, cancel := context.WithTimeoutCause(req.Context(), timeout, &TimeoutError{Duration: timeout})
	resp, err := base.RoundTrip(req.Clone(ctx))
	if err != nil {
		err = timeoutError(ctx, err)
		cancel()
		return resp, err
	}
	resp.Body = &responseBody{ReadCloser: resp.Body, ctx: ctx, cancel: cancel}
	return resp, nil
}

func timeoutError(ctx context.Context, err error) error {
	var timeout *TimeoutError
	if err != nil && errors.As(context.Cause(ctx), &timeout) {
		return timeout
	}
	return err
}

type responseBody struct {
	io.ReadCloser
	ctx    context.Context
	cancel context.CancelFunc
}

func (b *responseBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	err = timeoutError(b.ctx, err)
	if err != nil {
		b.cancel()
	}
	return n, err
}

func (b *responseBody) Close() error {
	b.cancel()
	return b.ReadCloser.Close()
}
