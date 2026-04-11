package handlers

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/kriswong/corticalstack/internal/persona"
)

// PersonaEditorPage renders the shared editor template for soul/user/memory.
// The route binds `name` via chi URL param.
func (h *Handler) PersonaEditorPage(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if !persona.IsValid(name) {
		http.Error(w, "unknown persona: "+name, http.StatusNotFound)
		return
	}

	content := ""
	if h.Persona != nil {
		content, _ = h.Persona.Get(persona.Name(name))
	}

	h.RenderPage(w, "persona_editor", map[string]interface{}{
		"Title":      titleFor(name),
		"ActivePage": name,
		"Name":       name,
		"File":       persona.Name(name).File(),
		"Content":    content,
		"Budget":     persona.Name(name).Budget(),
	})
}

// GetPersona returns the current content of a persona file as JSON.
func (h *Handler) GetPersona(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if !persona.IsValid(name) {
		http.Error(w, "unknown persona: "+name, http.StatusNotFound)
		return
	}
	if h.Persona == nil {
		http.Error(w, "persona loader not configured", http.StatusServiceUnavailable)
		return
	}
	content, err := h.Persona.Get(persona.Name(name))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{
		"name":    name,
		"file":    persona.Name(name).File(),
		"content": content,
		"budget":  persona.Name(name).Budget(),
	})
}

// SavePersona writes new content to a persona file. Accepts either a JSON
// body `{"content": "..."}` or a raw text/plain body.
func (h *Handler) SavePersona(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if !persona.IsValid(name) {
		http.Error(w, "unknown persona: "+name, http.StatusNotFound)
		return
	}
	if h.Persona == nil {
		http.Error(w, "persona loader not configured", http.StatusServiceUnavailable)
		return
	}

	ct := r.Header.Get("Content-Type")
	var content string
	if ct == "application/json" || ct == "application/json; charset=utf-8" {
		var req struct {
			Content string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
			return
		}
		content = req.Content
	} else {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "reading body: "+err.Error(), http.StatusBadRequest)
			return
		}
		content = string(raw)
	}

	if err := h.Persona.Set(persona.Name(name), content); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"status": "saved", "name": name})
}

func titleFor(name string) string {
	switch name {
	case "soul":
		return "SOUL — Extraction Style"
	case "user":
		return "USER — Profile"
	case "memory":
		return "MEMORY — Curated Index"
	}
	return "Persona"
}
