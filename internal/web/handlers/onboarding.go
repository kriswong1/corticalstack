package handlers

import (
	"net/http"
	"os"
	"strings"

	"github.com/kriswong/corticalstack/internal/persona"
)

// onboardingItem is one entry in the onboarding checklist.
type onboardingItem struct {
	ID         string `json:"id"`
	Label      string `json:"label"`
	Configured bool   `json:"configured"`
}

// OnboardingStatus returns the configuration state of the five onboarding
// items: Obsidian, Deepgram, Soul, User, Memory. Linear is excluded because
// it ships as "Coming Soon" this cycle.
func (h *Handler) OnboardingStatus(w http.ResponseWriter, r *http.Request) {
	items := []onboardingItem{
		{ID: "obsidian", Label: "Obsidian", Configured: h.isObsidianConfigured()},
		{ID: "deepgram", Label: "Deepgram", Configured: h.isDeepgramConfigured()},
		{ID: "soul", Label: "Soul", Configured: h.isPersonaConfigured(persona.NameSoul)},
		{ID: "user", Label: "User", Configured: h.isPersonaConfigured(persona.NameUser)},
		{ID: "memory", Label: "Memory", Configured: h.isPersonaConfigured(persona.NameMemory)},
	}

	count := 0
	for _, item := range items {
		if item.Configured {
			count++
		}
	}

	writeJSON(w, map[string]interface{}{
		"items":            items,
		"configured_count": count,
		"total":            len(items),
	})
}

func (h *Handler) isObsidianConfigured() bool {
	if h.Vault == nil {
		return false
	}
	info, err := os.Stat(h.Vault.Path())
	return err == nil && info.IsDir()
}

func (h *Handler) isDeepgramConfigured() bool {
	if h.Registry == nil {
		return false
	}
	dg := h.Registry.Get("deepgram")
	return dg != nil && dg.Configured()
}

func (h *Handler) isPersonaConfigured(name persona.Name) bool {
	if h.Persona == nil {
		return false
	}
	content, err := h.Persona.Get(name)
	if err != nil {
		return false
	}
	return hasPersonaBody(content)
}

// hasPersonaBody reports whether the persona file has meaningful content
// beyond default template boilerplate. A file with only frontmatter and
// empty section headings counts as unconfigured.
func hasPersonaBody(content string) bool {
	content = strings.TrimSpace(content)
	if content == "" {
		return false
	}
	// Strip YAML frontmatter.
	if strings.HasPrefix(content, "---") {
		end := strings.Index(content[3:], "---")
		if end >= 0 {
			content = strings.TrimSpace(content[end+6:])
		}
	}
	// Strip markdown headings and whitespace — if nothing remains,
	// the file is template-only.
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		return true
	}
	return false
}
