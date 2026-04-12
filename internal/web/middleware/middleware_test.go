package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// captureSlog installs a text handler writing to a buffer and returns
// the buffer. Caller must defer restoring the default logger.
func captureSlog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	return &buf
}

func TestRecoveryCatchesPanic(t *testing.T) {
	logBuf := captureSlog(t)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	})

	wrapped := Recovery(handler)
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	// Should not propagate the panic.
	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if !strings.Contains(rec.Body.String(), "boom") {
		t.Errorf("body missing panic message: %q", rec.Body.String())
	}
	if !strings.Contains(logBuf.String(), "panic in handler") {
		t.Errorf("log missing panic notice: %q", logBuf.String())
	}
}

func TestRecoveryPassesThrough(t *testing.T) {
	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusTeapot)
		w.Write([]byte("ok"))
	})

	wrapped := Recovery(handler)
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if !called {
		t.Error("inner handler not called")
	}
	if rec.Code != http.StatusTeapot {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusTeapot)
	}
	if rec.Body.String() != "ok" {
		t.Errorf("body = %q", rec.Body.String())
	}
}

func TestLoggerRecordsStatusAndPath(t *testing.T) {
	logBuf := captureSlog(t)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	wrapped := Logger(handler)
	req := httptest.NewRequest("POST", "/api/things", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	out := logBuf.String()
	for _, sub := range []string{"POST", "/api/things", "201"} {
		if !strings.Contains(out, sub) {
			t.Errorf("log missing %q in %q", sub, out)
		}
	}
}

func TestLoggerDefaultStatus200(t *testing.T) {
	logBuf := captureSlog(t)

	// Handler writes body without explicit WriteHeader — default 200.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello"))
	})

	wrapped := Logger(handler)
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if !strings.Contains(logBuf.String(), "200") {
		t.Errorf("expected 200 in log, got %q", logBuf.String())
	}
}

func TestStatusWriterCapturesWriteHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := &statusWriter{ResponseWriter: rec, status: 200}
	sw.WriteHeader(http.StatusForbidden)

	if sw.status != http.StatusForbidden {
		t.Errorf("status field = %d, want %d", sw.status, http.StatusForbidden)
	}
	if rec.Code != http.StatusForbidden {
		t.Errorf("underlying recorder code = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

// flushableRecorder wraps ResponseRecorder to satisfy http.Flusher.
type flushableRecorder struct {
	*httptest.ResponseRecorder
	flushed bool
}

func (f *flushableRecorder) Flush() {
	f.flushed = true
}

func TestStatusWriterFlushForwards(t *testing.T) {
	fr := &flushableRecorder{ResponseRecorder: httptest.NewRecorder()}
	sw := &statusWriter{ResponseWriter: fr, status: 200}
	sw.Flush()

	if !fr.flushed {
		t.Error("Flush did not forward to underlying Flusher")
	}
}

func TestStatusWriterFlushNoOpWithoutFlusher(t *testing.T) {
	// Plain ResponseRecorder *does* implement Flusher since Go 1.22,
	// so use a bare type that doesn't.
	nonFlusher := &nonFlushingWriter{ResponseWriter: httptest.NewRecorder()}
	sw := &statusWriter{ResponseWriter: nonFlusher, status: 200}
	// Should not panic.
	sw.Flush()
}

// nonFlushingWriter wraps a ResponseWriter without exposing Flusher.
type nonFlushingWriter struct {
	http.ResponseWriter
}

func TestRequestIDSetsHeader(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := ReqID(r.Context())
		if id == "" {
			t.Error("request ID missing from context")
		}
		w.Write([]byte(id))
	})

	wrapped := RequestID(handler)
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	hdr := rec.Header().Get("X-Request-ID")
	if hdr == "" {
		t.Error("X-Request-ID header not set")
	}
	if rec.Body.String() != hdr {
		t.Errorf("context ID %q != header ID %q", rec.Body.String(), hdr)
	}
}

func TestLoggerIncludesRequestID(t *testing.T) {
	logBuf := captureSlog(t)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := RequestID(Logger(inner))
	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	out := logBuf.String()
	if !strings.Contains(out, "request_id=") {
		t.Errorf("log missing request_id: %q", out)
	}
}
