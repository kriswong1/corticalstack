package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/kriswong/corticalstack/internal/agent"
	"github.com/kriswong/corticalstack/internal/config"
	"github.com/kriswong/corticalstack/internal/persona"
	"github.com/kriswong/corticalstack/internal/questions"
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
		var err error
		content, err = h.Persona.Get(persona.Name(name))
		if err != nil {
			slog.Warn("persona load failed", "name", name, "error", err)
		}
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

// SetupPersona handles POST /api/persona/setup — generates USER.md from
// a quick onboarding form (name, role, timezone, context, optional fields).
func (h *Handler) SetupPersona(w http.ResponseWriter, r *http.Request) {
	if h.Persona == nil {
		http.Error(w, "persona loader not configured", http.StatusServiceUnavailable)
		return
	}
	var req struct {
		Name      string   `json:"name"`
		Role      string   `json:"role"`
		Timezone  string   `json:"timezone"`
		Context   string   `json:"context"`
		Projects  []string `json:"projects,omitempty"`
		Platforms string   `json:"platforms,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Role) == "" {
		http.Error(w, "name and role are required", http.StatusBadRequest)
		return
	}

	var b strings.Builder
	b.WriteString("---\ntype: persona\nrole: user\npurpose: User profile and identity context\n---\n\n")
	b.WriteString("## Identity\n\n")
	b.WriteString(fmt.Sprintf("- **Name:** %s\n", req.Name))
	b.WriteString(fmt.Sprintf("- **Role:** %s\n", req.Role))
	if req.Timezone != "" {
		b.WriteString(fmt.Sprintf("- **Timezone:** %s\n", req.Timezone))
	}
	b.WriteString("\n## Context\n\n")
	if req.Context != "" {
		b.WriteString(req.Context + "\n")
	}
	if len(req.Projects) > 0 {
		b.WriteString("\n## Current Focus\n\n")
		for _, p := range req.Projects {
			if p = strings.TrimSpace(p); p != "" {
				b.WriteString(fmt.Sprintf("- %s\n", p))
			}
		}
	}
	if req.Platforms != "" {
		b.WriteString("\n## Platforms\n\n")
		b.WriteString(req.Platforms + "\n")
	}

	if err := h.Persona.Set(persona.NameUser, b.String()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"status": "saved", "name": "user"})
}

// QuestionsForPersonaEnhance handles POST /api/persona/{name}/enhance/questions.
// Returns clarifying questions Claude wants answered before improving the
// persona file.
func (h *Handler) QuestionsForPersonaEnhance(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if !persona.IsValid(name) {
		http.Error(w, "unknown persona: "+name, http.StatusNotFound)
		return
	}
	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}

	asker := questions.NewAsker("", config.ClaudeModel())
	goal := fmt.Sprintf("Improve and expand the %s persona file below (hard cap %d characters). Ask clarifying questions about what directions the user wants to push (more terse, more explicit, different tone, additional structure, etc.) — things we cannot infer from the current content alone.",
		strings.ToUpper(name), persona.Name(name).Budget())
	blocks := []questions.ContextBlock{
		{Heading: "Current persona content", Body: req.Content},
	}
	qs, err := asker.Ask(r.Context(), goal, blocks)
	if err != nil {
		slog.Error("persona enhance questions", "name", name, "error", err)
		http.Error(w, "questions: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{"questions": qs})
}

// EnhancePersona handles POST /api/persona/{name}/enhance — sends current
// content to Claude CLI for AI-assisted improvement.
func (h *Handler) EnhancePersona(w http.ResponseWriter, r *http.Request) {
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
		Content     string               `json:"content"`
		UserContext string               `json:"user_context,omitempty"`
		Questions   []questions.Question `json:"questions,omitempty"`
		Answers     []questions.Answer   `json:"answers,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}

	answerBlock := questions.FormatAnswers(req.Questions, req.Answers)

	prompt := fmt.Sprintf(`You are improving a CorticalStack persona file (%s).

%s## Current content
%s

## User context
%s

## Instructions
Improve and expand this persona file. Keep it under %d characters.
- For SOUL: refine extraction rules, tone, structure guidance, and never-do constraints.
- For USER: flesh out identity, role context, current focus, and platform details.
- For MEMORY: organize active decisions, load-bearing note pointers, and open questions.

Respond with ONLY the improved file content (including frontmatter). No explanation.`,
		strings.ToUpper(name), answerBlock, req.Content, req.UserContext, persona.Name(name).Budget())

	ag := &agent.Agent{
		Model:    config.ClaudeModel(),
		MaxTurns: 1,
	}
	enhanced, err := ag.RunSimple(r.Context(), prompt)
	if err != nil {
		slog.Error("persona enhance failed", "name", name, "error", err)
		http.Error(w, "Claude CLI error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{"content": strings.TrimSpace(enhanced)})
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
