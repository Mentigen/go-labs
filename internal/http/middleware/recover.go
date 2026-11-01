package middleware

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"runtime/debug"

	"go.opentelemetry.io/otel/trace"

	"github.com/Mentigen/go-labs/internal/http/domainerror"
)

var errPanic = errors.New("panic recovered")

type recoveryKey struct{}

type recoveryState struct {
	spanContext trace.SpanContext
}

func Recover(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			state := &recoveryState{}
			r = r.WithContext(context.WithValue(r.Context(), recoveryKey{}, state))

			defer func() {
				panicValue := recover()
				if panicValue == nil {
					return
				}

				if err, ok := panicValue.(error); ok && errors.Is(err, http.ErrAbortHandler) {
					panic(panicValue)
				}

				ctx := trace.ContextWithSpanContext(r.Context(), state.spanContext)
				logger.ErrorContext(ctx, "panic recovered",
					slog.Any("panic", panicValue),
					slog.String("stack", string(debug.Stack())),
				)

				status, body := domainerror.Problem(errPanic)
				domainerror.WriteProblem(w, r, status, body)
			}()

			next.ServeHTTP(w, r)
		})
	}
}
