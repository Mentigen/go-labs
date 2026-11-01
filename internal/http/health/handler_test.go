package health_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/Mentigen/go-labs/internal/http/health"
)

func TestHealthDoesNotTouchTheDatabase(t *testing.T) {
	t.Parallel()

	handler := health.New(NewMockpinger(gomock.NewController(t)), time.Second)
	recorder := httptest.NewRecorder()

	handler.Health(recorder, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/health", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
}

func TestReady(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		pingErr    error
		wantStatus int
	}{
		{"database is up", nil, http.StatusOK},
		{"database is down", errors.New("connection refused"), http.StatusServiceUnavailable},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			database := NewMockpinger(gomock.NewController(t))
			database.EXPECT().Ping(gomock.Any()).Return(test.pingErr)

			recorder := httptest.NewRecorder()

			health.New(database, time.Second).Ready(
				recorder,
				httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/ready", nil),
			)

			if recorder.Code != test.wantStatus {
				t.Errorf("status = %d, want %d", recorder.Code, test.wantStatus)
			}
		})
	}
}
