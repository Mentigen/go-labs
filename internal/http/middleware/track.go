package middleware

import (
	"net/http"
	"time"
)

type RouteResolver func(r *http.Request) string

type requestResult struct {
	status   int
	route    string
	duration time.Duration
}

type statusRecorder struct {
	http.ResponseWriter

	status int
}

func (s *statusRecorder) WriteHeader(status int) {
	if s.status == 0 {
		s.status = status
	}

	s.ResponseWriter.WriteHeader(status)
}

func (s *statusRecorder) Write(data []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}

	return s.ResponseWriter.Write(data)
}

func (s *statusRecorder) Unwrap() http.ResponseWriter {
	return s.ResponseWriter
}

func track(w http.ResponseWriter, r *http.Request, next http.Handler, route string, done func(requestResult)) {
	recorder := &statusRecorder{ResponseWriter: w}
	startedAt := time.Now()

	defer func() {
		panicValue := recover()

		status := recorder.status

		if panicValue != nil && status == 0 {
			status = http.StatusInternalServerError
		}

		if status == 0 {
			status = http.StatusOK
		}

		done(requestResult{status: status, route: route, duration: time.Since(startedAt)})

		if panicValue != nil {
			panic(panicValue)
		}
	}()

	next.ServeHTTP(recorder, r)
}
