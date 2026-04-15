package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/kriswong/corticalstack/internal/agent"
	"github.com/kriswong/corticalstack/internal/telemetry"
)

type stubUsage struct {
	recent     []agent.Invocation
	summary    telemetry.Summary
	recentErr  error
	summaryErr error
}

func (s *stubUsage) Recent(limit int) ([]agent.Invocation, error) {
	if s.recentErr != nil {
		return nil, s.recentErr
	}
	if limit > 0 && limit < len(s.recent) {
		return s.recent[:limit], nil
	}
	return s.recent, nil
}

func (s *stubUsage) Summary(window time.Duration) (telemetry.Summary, error) {
	if s.summaryErr != nil {
		return telemetry.Summary{}, s.summaryErr
	}
	return s.summary, nil
}

func newUsageTestHandler(u *stubUsage) (*Handler, *chi.Mux) {
	h := &Handler{Usage: u}
	r := chi.NewRouter()
	r.Get("/api/usage/recent", h.GetUsageRecent)
	r.Get("/api/usage/summary", h.GetUsageSummary)
	return h, r
}

func TestGetUsageRecentReturnsJSON(t *testing.T) {
	stub := &stubUsage{
		recent: []agent.Invocation{
			{SessionID: "a", Model: "claude-sonnet-4-5", InputTokens: 10, CostUSD: 0.01},
			{SessionID: "b", Model: "claude-sonnet-4-5", InputTokens: 20, CostUSD: 0.02},
		},
	}
	_, r := newUsageTestHandler(stub)

	req := httptest.NewRequest("GET", "/api/usage/recent", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var got []agent.Invocation
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 || got[0].SessionID != "a" || got[1].SessionID != "b" {
		t.Errorf("recent = %+v", got)
	}
}

func TestGetUsageRecentRespectsLimitQuery(t *testing.T) {
	stub := &stubUsage{
		recent: []agent.Invocation{{SessionID: "a"}, {SessionID: "b"}, {SessionID: "c"}},
	}
	_, r := newUsageTestHandler(stub)

	req := httptest.NewRequest("GET", "/api/usage/recent?limit=2", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var got []agent.Invocation
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("len = %d, want 2", len(got))
	}
}

func TestGetUsageRecentDefaultLimit(t *testing.T) {
	// 75 entries, no ?limit= → handler should default to 50.
	var invs []agent.Invocation
	for i := 0; i < 75; i++ {
		invs = append(invs, agent.Invocation{SessionID: "x"})
	}
	stub := &stubUsage{recent: invs}
	_, r := newUsageTestHandler(stub)

	req := httptest.NewRequest("GET", "/api/usage/recent", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var got []agent.Invocation
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 50 {
		t.Errorf("len = %d, want 50 (default)", len(got))
	}
}

func TestGetUsageRecentInvalidLimitReturns400(t *testing.T) {
	stub := &stubUsage{}
	_, r := newUsageTestHandler(stub)

	for _, q := range []string{"abc", "-1", "0"} {
		req := httptest.NewRequest("GET", "/api/usage/recent?limit="+q, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("limit=%q status = %d, want 400", q, rec.Code)
		}
	}
}

func TestGetUsageRecentEmptyReturnsEmptyArray(t *testing.T) {
	stub := &stubUsage{recent: nil}
	_, r := newUsageTestHandler(stub)

	req := httptest.NewRequest("GET", "/api/usage/recent", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	// Critical: nil slice must encode as [] not null so the React side
	// can call .map() without a guard.
	body := rec.Body.String()
	if body != "[]\n" {
		t.Errorf("body = %q, want %q", body, "[]\n")
	}
}

func TestGetUsageSummaryReturnsJSON(t *testing.T) {
	stub := &stubUsage{
		summary: telemetry.Summary{
			TotalCalls:   5,
			TotalCostUSD: 0.42,
			ByModel: map[string]telemetry.ModelTotals{
				"claude-sonnet-4-5": {Calls: 5, CostUSD: 0.42},
			},
		},
	}
	_, r := newUsageTestHandler(stub)

	req := httptest.NewRequest("GET", "/api/usage/summary?window=24h", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body: %s)", rec.Code, rec.Body.String())
	}
	var got telemetry.Summary
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.TotalCalls != 5 || got.TotalCostUSD != 0.42 {
		t.Errorf("summary = %+v", got)
	}
}

func TestGetUsageSummaryDefaultWindow(t *testing.T) {
	// No ?window= — handler should accept and call Summary(24h).
	stub := &stubUsage{summary: telemetry.Summary{TotalCalls: 1}}
	_, r := newUsageTestHandler(stub)

	req := httptest.NewRequest("GET", "/api/usage/summary", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d", rec.Code)
	}
}

func TestGetUsageSummaryInvalidWindowReturns400(t *testing.T) {
	stub := &stubUsage{}
	_, r := newUsageTestHandler(stub)

	for _, q := range []string{"abc", "-1h", "0s"} {
		req := httptest.NewRequest("GET", "/api/usage/summary?window="+q, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("window=%q status = %d, want 400", q, rec.Code)
		}
	}
}

func TestGetUsageRecentNilProviderReturns503(t *testing.T) {
	h := &Handler{} // no Usage
	req := httptest.NewRequest("GET", "/api/usage/recent", nil)
	rec := httptest.NewRecorder()
	h.GetUsageRecent(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
}

func TestGetUsageSummaryNilProviderReturns503(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequest("GET", "/api/usage/summary", nil)
	rec := httptest.NewRecorder()
	h.GetUsageSummary(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
}

func TestGetUsageRecentReaderErrorReturns503(t *testing.T) {
	stub := &stubUsage{recentErr: errors.New("disk gone")}
	_, r := newUsageTestHandler(stub)

	req := httptest.NewRequest("GET", "/api/usage/recent", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
}

func TestGetUsageSummaryReaderErrorReturns503(t *testing.T) {
	stub := &stubUsage{summaryErr: errors.New("disk gone")}
	_, r := newUsageTestHandler(stub)

	req := httptest.NewRequest("GET", "/api/usage/summary", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
}
