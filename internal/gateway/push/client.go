package push

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const (
	pushPath            = "/api/v1/push"
	kindRequestPosition = "REQUEST_POSITION"
)

var (
	ErrRejected    = errors.New("push service rejected the request")
	ErrUnavailable = errors.New("push service is unavailable")
)

type request struct {
	RequestID   string            `json:"request_id"`
	RecipientID uuid.UUID         `json:"recipient_id"`
	Kind        string            `json:"kind"`
	Data        map[string]string `json:"data,omitempty"`
}

type Client struct {
	http    *http.Client
	pushURL string
}

func New(baseURL string, timeout time.Duration, tp trace.TracerProvider, propagator propagation.TextMapPropagator) *Client {
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}).DialContext,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   timeout,
		ResponseHeaderTimeout: timeout,
	}

	return &Client{
		http: &http.Client{
			Timeout: timeout,
			Transport: otelhttp.NewTransport(transport,
				otelhttp.WithTracerProvider(tp),
				otelhttp.WithMeterProvider(noop.NewMeterProvider()),
				otelhttp.WithPropagators(propagator),
			),
		},
		pushURL: baseURL + pushPath,
	}
}

func (c *Client) RequestPosition(ctx context.Context, driverID, tripID uuid.UUID) error {
	body, err := json.Marshal(request{
		RequestID:   uuid.NewString(),
		RecipientID: driverID,
		Kind:        kindRequestPosition,
		Data:        map[string]string{"trip_id": tripID.String()},
	})
	if err != nil {
		return fmt.Errorf("marshal push request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.pushURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build push request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("send push request: %w", err)
	}

	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	switch {
	case resp.StatusCode >= http.StatusInternalServerError:
		return fmt.Errorf("%w: status %d", ErrUnavailable, resp.StatusCode)
	case resp.StatusCode >= http.StatusBadRequest:
		return fmt.Errorf("%w: status %d", ErrRejected, resp.StatusCode)
	case resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices:
		return fmt.Errorf("%w: unexpected status %d", ErrUnavailable, resp.StatusCode)
	}

	return nil
}

func (c *Client) Close() {
	c.http.CloseIdleConnections()
}
