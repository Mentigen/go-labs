package httpserver

import (
	"net/http"

	"github.com/Mentigen/go-labs/internal/config"
)

func New(addr string, handler http.Handler, timeouts config.HTTPTimeouts) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadTimeout:       timeouts.Read,
		ReadHeaderTimeout: timeouts.ReadHeader,
		WriteTimeout:      timeouts.Write,
		IdleTimeout:       timeouts.Idle,
	}
}
