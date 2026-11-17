package httpserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.uber.org/mock/gomock"

	tripdomain "github.com/Mentigen/go-labs/internal/domain/trip"
	"github.com/Mentigen/go-labs/internal/generated"
	httpserver "github.com/Mentigen/go-labs/internal/http"
	"github.com/Mentigen/go-labs/internal/http/health"
	createhandler "github.com/Mentigen/go-labs/internal/http/trip/create"
	finishhandler "github.com/Mentigen/go-labs/internal/http/trip/finish"
	gethandler "github.com/Mentigen/go-labs/internal/http/trip/get"
	"github.com/Mentigen/go-labs/internal/infra/telemetry"
	"github.com/Mentigen/go-labs/internal/model"
)

const (
	tripsPath          = "/api/v1/trips"
	codeInvalidRequest = "invalid_request"
	contentTypeJSON    = "application/json"
	levelError         = "ERROR"
)

var tripID = uuid.MustParse("11111111-1111-1111-1111-111111111111")

//go:generate go tool mockgen -source trip/create/contract.go -destination create_mocks_test.go -package httpserver_test -mock_names usecase=MockCreateUsecase
//go:generate go tool mockgen -source trip/get/contract.go -destination trip_mocks_test.go -package httpserver_test -mock_names usecase=MockTripUsecase
//go:generate go tool mockgen -source health/contract.go -destination health_mocks_test.go -package httpserver_test

type harness struct {
	handler http.Handler
	spans   *tracetest.InMemoryExporter
	reader  *sdkmetric.ManualReader
	logs    *bytes.Buffer
}

func newHarness(t *testing.T,
	create func(model.CreateTripInput) (model.CreateTripResult, error),
	get, finish func() (tripdomain.Trip, error),
) *harness {
	t.Helper()

	ctrl := gomock.NewController(t)
	createMock := NewMockCreateUsecase(ctrl)
	createMock.EXPECT().Execute(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, input model.CreateTripInput) (model.CreateTripResult, error) {
			return create(input)
		},
	).AnyTimes()
	getMock := NewMockTripUsecase(ctrl)
	getMock.EXPECT().Execute(gomock.Any(), gomock.Any()).DoAndReturn(
		func(context.Context, uuid.UUID) (tripdomain.Trip, error) { return get() },
	).AnyTimes()
	finishMock := NewMockTripUsecase(ctrl)
	finishMock.EXPECT().Execute(gomock.Any(), gomock.Any()).DoAndReturn(
		func(context.Context, uuid.UUID) (tripdomain.Trip, error) { return finish() },
	).AnyTimes()
	pinger := NewMockpinger(ctrl)
	pinger.EXPECT().Ping(gomock.Any()).Return(nil).AnyTimes()

	spans := tracetest.NewInMemoryExporter()
	tracerProvider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(spans))
	reader := sdkmetric.NewManualReader()
	meterProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	logs := &bytes.Buffer{}
	logger := telemetry.NewLogger(logs, 0, "trip-service")

	handler, err := httpserver.NewRouter(httpserver.Options{
		Handlers: httpserver.Handlers{
			Create: createhandler.New(createMock, logger),
			Get:    gethandler.New(getMock, logger),
			Finish: finishhandler.New(finishMock, logger),
			Health: health.New(pinger, time.Second),
		},
		Logger:         logger,
		TracerProvider: tracerProvider,
		MeterProvider:  meterProvider,
		Propagator:     propagation.TraceContext{},
	})
	require.NoError(t, err, "build router: %v", err)

	return &harness{handler: handler, spans: spans, reader: reader, logs: logs}
}

func defaultHarness(t *testing.T) *harness {
	t.Helper()

	trip := tripdomain.Trip{ID: tripID, Status: tripdomain.StatusActive}

	return newHarness(t,
		func(model.CreateTripInput) (model.CreateTripResult, error) {
			return model.CreateTripResult{Trip: trip, Created: true}, nil
		},
		func() (tripdomain.Trip, error) { return trip, nil },
		func() (tripdomain.Trip, error) { return trip, nil },
	)
}

func (h *harness) do(method, path, contentType, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequestWithContext(context.Background(), method, path, strings.NewReader(body))
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}

	recorder := httptest.NewRecorder()
	h.handler.ServeHTTP(recorder, request)

	return recorder
}

const validBody = `{
	"user_id": "22222222-2222-2222-2222-222222222222",
	"driver_id": "33333333-3333-3333-3333-333333333333",
	"start_point": {"latitude": 55.75, "longitude": 37.61},
	"end_point": {"latitude": 55.8, "longitude": 37.7},
	"price": 500
}`

func TestValidRequestsPassThrough(t *testing.T) {
	t.Parallel()

	h := defaultHarness(t)

	tests := []struct {
		name        string
		method      string
		path        string
		contentType string
		body        string
		want        int
	}{
		{"create", http.MethodPost, tripsPath, contentTypeJSON, validBody, http.StatusCreated},
		{"create with charset", http.MethodPost, tripsPath, "application/json; charset=utf-8", validBody, http.StatusCreated},
		{"get", http.MethodGet, "/api/v1/trips/" + tripID.String(), "", "", http.StatusOK},
		{"finish", http.MethodPost, "/api/v1/trips/" + tripID.String() + "/finish", "", "", http.StatusOK},
		{"health", http.MethodGet, "/health", "", "", http.StatusOK},
		{"ready", http.MethodGet, "/ready", "", "", http.StatusOK},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			recorder := h.do(test.method, test.path, test.contentType, test.body)

			require.Equal(t, test.want, recorder.Code, "status = %d, want %d: %s", recorder.Code, test.want, recorder.Body.String())
		})
	}
}

func TestInvalidRequestsAreRejectedWithProblemJSON(t *testing.T) {
	t.Parallel()

	h := defaultHarness(t)

	tests := []struct {
		name        string
		method      string
		path        string
		contentType string
		body        string
		wantStatus  int
		wantCode    string
	}{
		{"broken json", http.MethodPost, tripsPath, contentTypeJSON, `{"user_id":`, 400, codeInvalidRequest},
		{"empty body", http.MethodPost, tripsPath, contentTypeJSON, ``, 400, codeInvalidRequest},
		{"missing field", http.MethodPost, tripsPath, contentTypeJSON, `{"price": 1}`, 400, codeInvalidRequest},
		{"null start point", http.MethodPost, tripsPath, contentTypeJSON, strings.Replace(validBody, `"start_point": {"latitude": 55.75, "longitude": 37.61}`, `"start_point": null`, 1), 400, codeInvalidRequest},
		{"latitude out of range", http.MethodPost, tripsPath, contentTypeJSON, strings.Replace(validBody, "55.75", "200", 1), 400, codeInvalidRequest},
		{"negative price", http.MethodPost, tripsPath, contentTypeJSON, strings.Replace(validBody, `"price": 500`, `"price": -5`, 1), 400, codeInvalidRequest},
		{"extra field", http.MethodPost, tripsPath, contentTypeJSON, strings.Replace(validBody, `"price": 500`, `"price": 500, "tip": 1`, 1), 400, codeInvalidRequest},
		{"missing content type", http.MethodPost, tripsPath, "", validBody, 400, codeInvalidRequest},
		{"form content type", http.MethodPost, tripsPath, "application/x-www-form-urlencoded", validBody, 400, codeInvalidRequest},
		{"trip id is not a uuid", http.MethodGet, "/api/v1/trips/not-a-uuid", "", "", 400, codeInvalidRequest},
		{"unknown path", http.MethodGet, "/api/v1/unknown", "", "", 404, "not_found"},
		{"wrong method", http.MethodDelete, tripsPath, "", "", 405, "method_not_allowed"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			recorder := h.do(test.method, test.path, test.contentType, test.body)

			require.Equal(t, test.wantStatus, recorder.Code, "status = %d, want %d: %s", recorder.Code, test.wantStatus, recorder.Body.String())

			{
				got := recorder.Header().Get("Content-Type")
				assert.Equal(t, "application/problem+json", got)
			}

			var problem generated.Problem
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &problem))

			assert.Equal(t, test.wantCode, problem.Code, "code = %q, want %q", problem.Code, test.wantCode)
		})
	}
}

func TestPanicInHandlerIsRecovered(t *testing.T) {
	t.Parallel()

	healthy := tripdomain.Trip{ID: tripID, Status: tripdomain.StatusActive}

	h := newHarness(t,
		func(model.CreateTripInput) (model.CreateTripResult, error) {
			panic("nil pointer somewhere deep")
		},
		func() (tripdomain.Trip, error) { return healthy, nil },
		func() (tripdomain.Trip, error) { return healthy, nil },
	)

	recorder := h.do(http.MethodPost, tripsPath, contentTypeJSON, validBody)

	require.Equal(t, http.StatusInternalServerError, recorder.Code, "status = %d, want 500", recorder.Code)

	assert.NotContains(t, recorder.Body.String(), "nil pointer", "response leaks the panic value: %s", recorder.Body.String())

	var problem generated.Problem
	{
		err := json.Unmarshal(recorder.Body.Bytes(), &problem)
		assert.NoError(t, err)
		assert.Equal(t, "internal_error", problem.Code)
	}

	records := decodeLogs(t, h.logs)

	panicRecord := findRecord(records, "panic recovered")
	require.NotNil(t, panicRecord)
	assert.Equal(t, levelError, panicRecord["level"])
	assert.NotEqual(t, "", panicRecord["stack"])

	requestRecord := findRecord(records, "request handled")
	require.NotNil(t, requestRecord)
	assert.Equal(t, levelError, requestRecord["level"])
	assert.Equal(t, float64(500), requestRecord["status"])

	spans := h.spans.GetSpans()
	require.Len(t, spans, 1, "expected one failed HTTP span, got %d", len(spans))

	span := spans[0]
	assert.Equal(t, codes.Error, span.Status.Code, "panic span status = %v, want Error", span.Status)

	attrs := make(map[string]any)
	for _, attr := range span.Attributes {
		attrs[string(attr.Key)] = attr.Value.AsInterface()
	}
	assert.Equal(t, int64(500), attrs["http.response.status_code"])
	assert.Equal(t, "500", attrs["error.type"])

	for _, record := range []map[string]any{panicRecord, requestRecord} {
		assert.Equal(t, span.SpanContext.TraceID().String(), record["trace_id"])
		assert.Equal(t, span.SpanContext.SpanID().String(), record["span_id"])
	}

	{
		after := h.do(http.MethodGet, "/health", "", "")
		assert.Equal(t, http.StatusOK, after.Code)
	}
}

func TestRequestLogUsesRouteTemplateAndCarriesTraceIDs(t *testing.T) {
	t.Parallel()

	h := defaultHarness(t)

	h.do(http.MethodGet, "/api/v1/trips/"+tripID.String(), "", "")

	records := decodeLogs(t, h.logs)
	require.Len(t, records, 1, "expected exactly one record per request, got %d: %v", len(records), records)

	record := records[0]

	assert.Equal(t, "request handled", record["msg"])
	assert.Equal(t, "INFO", record["level"])

	assert.Equal(t, "/api/v1/trips/{tripId}", record["route"], "route = %v, want the template", record["route"])

	assert.Equal(t, "GET", record["method"])
	assert.Equal(t, float64(200), record["status"])
	assert.Equal(t, "trip-service", record["service"])

	{
		_, ok := record["duration_ms"]
		assert.True(t, ok)
	}

	spans := h.spans.GetSpans()
	require.Len(t, spans, 1, "expected one span, got %d", len(spans))

	assert.Equal(t, spans[0].SpanContext.TraceID().String(), record["trace_id"])
	assert.Equal(t, spans[0].SpanContext.SpanID().String(), record["span_id"])
}

func TestRequestLogLevelFollowsStatus(t *testing.T) {
	t.Parallel()

	h := newHarness(t,
		func(model.CreateTripInput) (model.CreateTripResult, error) {
			return model.CreateTripResult{}, tripdomain.ErrDriverBusy
		},
		func() (tripdomain.Trip, error) { return tripdomain.Trip{}, assertionError{} },
		func() (tripdomain.Trip, error) { return tripdomain.Trip{}, tripdomain.ErrTripNotFound },
	)

	h.do(http.MethodPost, tripsPath, contentTypeJSON, validBody)
	h.do(http.MethodGet, "/api/v1/trips/"+tripID.String(), "", "")

	statusToLevel := map[float64]string{}

	for _, record := range decodeLogs(t, h.logs) {
		if record["msg"] == "request handled" {
			statusToLevel[record["status"].(float64)] = record["level"].(string)
		}
	}

	assert.Equal(t, "INFO", statusToLevel[409], "4xx must be logged at INFO, got %q", statusToLevel[409])

	assert.Equal(t, levelError, statusToLevel[500], "5xx must be logged at ERROR, got %q", statusToLevel[500])
}

type assertionError struct{}

func (assertionError) Error() string { return "internal failure" }

func TestServerSpanIsNamedAfterMethodAndRouteTemplate(t *testing.T) {
	t.Parallel()

	h := defaultHarness(t)

	h.do(http.MethodGet, "/api/v1/trips/"+tripID.String(), "", "")
	h.do(http.MethodPost, tripsPath, contentTypeJSON, validBody)

	names := map[string]bool{}

	for _, span := range h.spans.GetSpans() {
		names[span.Name] = true
	}

	for _, want := range []string{"GET /api/v1/trips/{tripId}", "POST /api/v1/trips"} {
		assert.True(t, names[want], "span %q not found, have %v", want, names)
	}

	for name := range names {
		assert.NotContains(t, name, tripID.String(), "span name contains the raw id: %q", name)
	}
}

func TestMetricsUseRouteTemplateAndCoverRequiredInstruments(t *testing.T) {
	t.Parallel()

	h := defaultHarness(t)

	h.do(http.MethodGet, "/api/v1/trips/"+tripID.String(), "", "")
	h.do(http.MethodGet, "/api/v1/trips/"+uuid.NewString(), "", "")

	var collected metricdata.ResourceMetrics
	require.NoError(t, h.reader.Collect(context.Background(), &collected))

	routes := map[string]bool{}
	instruments := map[string]bool{}

	for _, scope := range collected.ScopeMetrics {
		for _, instrument := range scope.Metrics {
			instruments[instrument.Name] = true

			histogram, ok := instrument.Data.(metricdata.Histogram[float64])
			if !ok {
				continue
			}

			for _, point := range histogram.DataPoints {
				if route, found := point.Attributes.Value("http.route"); found {
					routes[route.AsString()] = true
				}

				{
					_, found := point.Attributes.Value("http.response.status_code")
					assert.True(t, found)
				}
			}
		}
	}

	assert.True(t, instruments["http.server.request.duration"] || !instruments["http.server.active_requests"], "required instruments are missing, have %v", instruments)

	assert.EqualValues(t, 1, len(routes))
	assert.True(t, routes["/api/v1/trips/{tripId}"])
}

func decodeLogs(t *testing.T, buffer *bytes.Buffer) []map[string]any {
	t.Helper()

	var records []map[string]any

	for line := range strings.SplitSeq(strings.TrimSpace(buffer.String()), "\n") {
		if line == "" {
			continue
		}

		record := map[string]any{}
		require.NoError(t, json.Unmarshal([]byte(line), &record))

		records = append(records, record)
	}

	return records
}

func findRecord(records []map[string]any, msg string) map[string]any {
	for _, record := range records {
		if record["msg"] == msg {
			return record
		}
	}

	return nil
}
