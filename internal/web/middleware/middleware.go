// Package middleware contains HTTP middleware used by the web server.
package middleware

import (
	"fmt"
	"log"
	"net/http"
	"runtime/debug"
	"time"
)

// Recovery catches panics in handlers and returns HTTP 500 instead of
// crashing the server.
func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("panic in handler: %v\n%s", err, debug.Stack())
				http.Error(w, fmt.Sprintf("internal server error: %v", err), http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// Logger logs each HTTP request with method, path, status, and duration.
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(sw, r)
		log.Printf("%s %s → %d (%s)", r.Method, r.URL.Path, sw.status, time.Since(start))
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
}

// Flush implements http.Flusher so SSE handlers can stream.
func (sw *statusWriter) Flush() {
	if f, ok := sw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
