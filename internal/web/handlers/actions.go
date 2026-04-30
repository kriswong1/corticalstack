package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/kriswong/corticalstack/internal/actions"
	"github.com/kriswong/corticalstack/internal/config"
	"github.com/kriswong/corticalstack/internal/integrations/linear"
)

// kickLinearSync fires a non-blocking Action sync if Linear is
// configured. Errors are logged, never returned — Action mutations
// must succeed regardless of Linear availability.
func (h *Handler) kickLinearSync(actionID string) {
	li := h.linearIntegration()
	if li == nil || !li.Configured() || li.CurrentTeamKey() == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		stores := linear.SyncStores{
			Projects:    h.Projects,
			Initiatives: h.Initiatives,
			PRDs:        h.PRDs,
			Actions:     h.Actions,
		}
		orch := linear.NewOrchestrator(li.Client, stores, li.CurrentTeamKey())
		if err := orch.SyncAction(ctx, actionID); err != nil {
			slog.Warn("linear: action sync failed", "action_id", actionID, "error", err)
		}
	}()
}

// ListActions returns all tracked actions, optionally filtered by ?status=.
func (h *Handler) ListActions(w http.ResponseWriter, r *http.Request) {
	if h.Actions == nil {
		writeJSON(w, []actions.Action{})
		return
	}
	status := r.URL.Query().Get("status")
	var out []*actions.Action
	if status != "" {
		out = h.Actions.ListByStatus(actions.Status(status))
	} else {
		out = h.Actions.List()
	}
	writeJSON(w, out)
}

// CreateAction handles POST /api/actions for manual quick-add from the
// dashboard. Required: description (the typed line). Optional: title,
// project_ids, my_day, starred, parent_id, deadline, priority, effort,
// context. Owner defaults to "me" so unowned UI-created tasks don't
// render as "TBD" — quick-add tasks belong to the user typing them.
func (h *Handler) CreateAction(w http.ResponseWriter, r *http.Request) {
	if h.Actions == nil {
		http.Error(w, "action store not configured", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		Title       string             `json:"title"`
		Description string             `json:"description"`
		Owner       string             `json:"owner"`
		Deadline    string             `json:"deadline"`
		Priority    actions.Priority   `json:"priority"`
		Effort      actions.Effort     `json:"effort"`
		Context     string             `json:"context"`
		ProjectIDs  []string           `json:"project_ids"`
		MyDay       bool               `json:"my_day"`
		Starred     bool               `json:"starred"`
		ParentID    string             `json:"parent_id"`
		Status      actions.Status     `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	if body.Description == "" && body.Title == "" {
		http.Error(w, "description or title required", http.StatusBadRequest)
		return
	}
	owner := body.Owner
	if owner == "" {
		owner = "me"
	}
	status := body.Status
	if status == "" {
		// Manually-created tasks skip the Inbox triage step that
		// pipeline-extracted tasks need — the user typed it themselves
		// so they've already triaged it. Land in "next".
		status = actions.StatusNext
	}
	a := &actions.Action{
		Title:       body.Title,
		Description: body.Description,
		Owner:       owner,
		Deadline:    body.Deadline,
		Status:      status,
		Priority:    body.Priority,
		Effort:      body.Effort,
		Context:     body.Context,
		ProjectIDs:  body.ProjectIDs,
		MyDay:       body.MyDay,
		Starred:     body.Starred,
		ParentID:    body.ParentID,
	}
	created, err := h.Actions.Upsert(a)
	if err != nil {
		internalError(w, "action.create", err)
		return
	}
	if err := h.Actions.Sync(created); err != nil {
		internalError(w, "action.sync_after_create", err)
		return
	}
	h.kickLinearSync(created.ID)
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, created)
}

// ActionCounts returns a status → count map.
func (h *Handler) ActionCounts(w http.ResponseWriter, r *http.Request) {
	if h.Actions == nil {
		writeJSON(w, map[string]int{})
		return
	}
	writeJSON(w, h.Actions.CountByStatus())
}

// SetActionStatus handles POST /api/actions/{id}/status with JSON { status }.
func (h *Handler) SetActionStatus(w http.ResponseWriter, r *http.Request) {
	if h.Actions == nil {
		http.Error(w, "action store not configured", http.StatusServiceUnavailable)
		return
	}
	id := chi.URLParam(r, "id")
	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	if !actions.IsValid(body.Status) {
		http.Error(w, "invalid status: "+body.Status, http.StatusBadRequest)
		return
	}
	// HI-02: push the WIP-limit check into the store so the count + status
	// change happen under a single lock. Previously, a separate CountByStatus
	// + SetStatus pair could let N concurrent requests all pass the check
	// and all transition to "doing", overshooting the cap.
	target := actions.MigrateStatus(actions.Status(body.Status))
	limit := config.WIPLimit()
	updated, err := h.Actions.SetStatusWithLimit(id, target, limit)
	if errors.Is(err, actions.ErrWIPLimit) {
		http.Error(w, fmt.Sprintf("WIP limit reached: max %d items in 'doing'", limit), http.StatusConflict)
		return
	}
	if err != nil {
		// "action not found: <id>" is user-actionable and contains only the
		// ID the client already sent, so it's safe to echo back.
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	// Propagate the change to every markdown location.
	if err := h.Actions.Sync(updated); err != nil {
		internalError(w, "action.sync_after_status", err)
		return
	}
	h.kickLinearSync(updated.ID)
	writeJSON(w, updated)
}

// UpdateAction handles PUT /api/actions/{id} with a partial JSON body.
func (h *Handler) UpdateAction(w http.ResponseWriter, r *http.Request) {
	if h.Actions == nil {
		http.Error(w, "action store not configured", http.StatusServiceUnavailable)
		return
	}
	id := chi.URLParam(r, "id")
	var patch actions.ActionPatch
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	// HI-02: atomic check+update under the store's lock.
	limit := config.WIPLimit()
	updated, err := h.Actions.UpdateWithLimit(id, patch, limit)
	if errors.Is(err, actions.ErrWIPLimit) {
		http.Error(w, fmt.Sprintf("WIP limit reached: max %d items in 'doing'", limit), http.StatusConflict)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if err := h.Actions.Sync(updated); err != nil {
		internalError(w, "action.sync_after_update", err)
		return
	}
	h.kickLinearSync(updated.ID)
	writeJSON(w, updated)
}

// ReconcileActions handles POST /api/actions/reconcile.
func (h *Handler) ReconcileActions(w http.ResponseWriter, r *http.Request) {
	if h.Actions == nil {
		http.Error(w, "action store not configured", http.StatusServiceUnavailable)
		return
	}
	res, err := h.Actions.Reconcile()
	if err != nil {
		internalError(w, "action.reconcile", err)
		return
	}
	writeJSON(w, res)
}

// ActionsPage renders the global action tracker view.
func (h *Handler) ActionsPage(w http.ResponseWriter, r *http.Request) {
	var items []*actions.Action
	var counts map[actions.Status]int
	if h.Actions != nil {
		items = h.Actions.List()
		counts = h.Actions.CountByStatus()
	}
	h.RenderPage(w, "actions", map[string]interface{}{
		"Title":      "Actions",
		"ActivePage": "actions",
		"Items":      items,
		"Counts":     counts,
		"Statuses":   actions.AllStatuses(),
	})
}
