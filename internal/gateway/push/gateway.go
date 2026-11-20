package push

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	pushapi "github.com/Mentigen/go-labs/internal/generated/push"
)

type HTTPError struct{ Status int }

func (e *HTTPError) Error() string { return fmt.Sprintf("push service returned HTTP %d", e.Status) }

type Gateway struct {
	client    *http.Client
	transport *http.Transport
	endpoint  string
	newID     func() uuid.UUID
}

func New(
	baseURL string,
	timeout time.Duration,
	tracer trace.TracerProvider,
	propagator propagation.TextMapPropagator,
	newID func() uuid.UUID,
) (*Gateway, error) {
	endpoint, err := url.JoinPath(baseURL, "api/v1/push")
	if err != nil {
		return nil, fmt.Errorf("push endpoint: %w", err)
	}
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: timeout, KeepAlive: timeout}).DialContext,
		TLSHandshakeTimeout:   timeout,
		ResponseHeaderTimeout: timeout,
		IdleConnTimeout:       timeout,
	}
	client := &http.Client{
		Timeout:       timeout,
		Transport:     otelhttp.NewTransport(transport, otelhttp.WithTracerProvider(tracer), otelhttp.WithPropagators(propagator)),
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	return &Gateway{client: client, transport: transport, endpoint: endpoint, newID: newID}, nil
}

func (g *Gateway) RequestPosition(ctx context.Context, tripID, driverID uuid.UUID) error {
	data := map[string]string{"trip_id": tripID.String()}
	payload := pushapi.PushRequest{
		RequestId:   g.newID().String(),
		RecipientId: driverID,
		Kind:        pushapi.REQUESTPOSITION,
		Data:        &data,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode position request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, g.endpoint, bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("build position request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := g.client.Do(request)
	if err != nil {
		return fmt.Errorf("send position request: %w", err)
	}
	defer response.Body.Close()
	_, readErr := io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
	if response.StatusCode != http.StatusAccepted {
		return &HTTPError{Status: response.StatusCode}
	}
	if readErr != nil {
		return fmt.Errorf("read push response: %w", readErr)
	}
	return nil
}

func (g *Gateway) Close() { g.transport.CloseIdleConnections() }
