package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/kriswong/corticalstack/internal/meetings"
)

type stubMeetings struct {
	list []*meetings.Meeting
	err  error
}

func (s *stubMeetings) List() ([]*meetings.Meeting, error) {
	return s.list, s.err
}

func newMeetingsTestHandler(m meetingsProvider) (*Handler, *chi.Mux) {
	h := &Handler{Meetings: m}
	r := chi.NewRouter()
	r.Get("/api/meetings", h.ListMeetings)
	return h, r
}

func TestListMeetingsReturnsArray(t *testing.T) {
	stub := &stubMeetings{list: []*meetings.Meeting{
		{ID: "m1", Title: "Kickoff", Stage: meetings.StageTranscript},
		{ID: "m2", Title: "Retro", Stage: meetings.StageSummary},
	}}
	_, r := newMeetingsTestHandler(stub)

	req := httptest.NewRequest("GET", "/api/meetings", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var got []*meetings.Meeting
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 || got[0].ID != "m1" || got[1].Stage != meetings.StageSummary {
		t.Errorf("got = %+v", got)
	}
}

func TestListMeetingsEmptyReturnsEmptyArray(t *testing.T) {
	stub := &stubMeetings{list: nil}
	_, r := newMeetingsTestHandler(stub)

	req := httptest.NewRequest("GET", "/api/meetings", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if rec.Body.String() != "[]\n" {
		t.Errorf("body = %q, want %q", rec.Body.String(), "[]\n")
	}
}

func TestListMeetingsNilProviderReturnsEmptyArray(t *testing.T) {
	// Nil provider must return an empty array, not 503 — the dashboard
	// expects a parseable response on a fresh CorticalStack install.
	h := &Handler{}
	req := httptest.NewRequest("GET", "/api/meetings", nil)
	rec := httptest.NewRecorder()
	h.ListMeetings(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != "[]\n" {
		t.Errorf("body = %q", rec.Body.String())
	}
}

func TestListMeetingsStoreErrorReturns503(t *testing.T) {
	stub := &stubMeetings{err: errors.New("disk gone")}
	_, r := newMeetingsTestHandler(stub)

	req := httptest.NewRequest("GET", "/api/meetings", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
}
