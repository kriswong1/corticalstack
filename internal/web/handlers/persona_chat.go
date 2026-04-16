package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/kriswong/corticalstack/internal/config"
	"github.com/kriswong/corticalstack/internal/persona"
)

// StartPersonaChat creates a new chat session and returns the first
// assistant message. POST /api/persona/{name}/chat/start
func (h *Handler) StartPersonaChat(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if !persona.IsValid(name) {
		http.Error(w, "unknown persona: "+name, http.StatusNotFound)
		return
	}
	if h.PersonaChatStore == nil {
		http.Error(w, "chat not configured", http.StatusServiceUnavailable)
		return
	}

	session, err := persona.StartChat(r.Context(), persona.Name(name), config.ClaudeModel(), h.chatWorkingDir())
	if err != nil {
		internalError(w, "persona.chat.start", err)
		return
	}

	h.PersonaChatStore.Put(session)

	last := session.Messages[len(session.Messages)-1]
	writeJSON(w, map[string]interface{}{
		"session_id": session.ID,
		"message":    last,
		"turn":       session.TurnCount,
		"max_turns":  session.MaxTurns,
		"done":       session.Done,
	})
}

// ContinuePersonaChat sends the user's input and returns Claude's response.
// POST /api/persona/{name}/chat/continue
func (h *Handler) ContinuePersonaChat(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if !persona.IsValid(name) {
		http.Error(w, "unknown persona: "+name, http.StatusNotFound)
		return
	}

	var req struct {
		SessionID string `json:"session_id"`
		Input     string `json:"input"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	session, err := h.PersonaChatStore.MustGet(req.SessionID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	done, err := persona.ContinueChat(r.Context(), session, req.Input, config.ClaudeModel(), h.chatWorkingDir())
	if err != nil {
		internalError(w, "persona.chat.continue", err)
		return
	}
	h.PersonaChatStore.Touch(session.ID)

	last := session.Messages[len(session.Messages)-1]
	writeJSON(w, map[string]interface{}{
		"message":   last,
		"turn":      session.TurnCount,
		"max_turns": session.MaxTurns,
		"done":      done,
		"result":    session.Result,
	})
}

// FinishPersonaChat forces the chat to generate the persona file early.
// POST /api/persona/{name}/chat/done
func (h *Handler) FinishPersonaChat(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if !persona.IsValid(name) {
		http.Error(w, "unknown persona: "+name, http.StatusNotFound)
		return
	}

	var req struct {
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	session, err := h.PersonaChatStore.MustGet(req.SessionID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	if err := persona.FinishChat(r.Context(), session, config.ClaudeModel(), h.chatWorkingDir()); err != nil {
		internalError(w, "persona.chat.done", err)
		return
	}

	writeJSON(w, map[string]interface{}{
		"content": session.Result,
		"done":    true,
	})
}

// AcceptPersonaChat saves the generated persona file and cleans up the session.
// POST /api/persona/{name}/chat/accept
func (h *Handler) AcceptPersonaChat(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if !persona.IsValid(name) {
		http.Error(w, "unknown persona: "+name, http.StatusNotFound)
		return
	}
	if h.Persona == nil {
		http.Error(w, "persona loader not configured", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	session, err := h.PersonaChatStore.MustGet(req.SessionID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if session.Result == "" {
		http.Error(w, "no generated content to accept", http.StatusBadRequest)
		return
	}

	if err := h.Persona.Set(persona.Name(name), session.Result); err != nil {
		internalError(w, "persona.chat.accept", err)
		return
	}

	h.PersonaChatStore.Delete(session.ID)

	writeJSON(w, map[string]interface{}{
		"status": "saved",
		"name":   name,
	})
}

func (h *Handler) chatWorkingDir() string {
	if h.Vault != nil {
		return h.Vault.Path()
	}
	return "."
}
