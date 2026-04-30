package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/kriswong/corticalstack/internal/config"
	"github.com/kriswong/corticalstack/internal/integrations"
)

// --- Obsidian ---

// TestObsidian validates that a vault path exists and is a directory.
// POST /api/integrations/obsidian/test  {vault_path: string}
func (h *Handler) TestObsidian(w http.ResponseWriter, r *http.Request) {
	var req struct {
		VaultPath string `json:"vault_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	p := strings.TrimSpace(req.VaultPath)
	if p == "" {
		writeJSON(w, map[string]interface{}{"ok": false, "error": "vault path is required"})
		return
	}
	info, err := os.Stat(p)
	if err != nil {
		writeJSON(w, map[string]interface{}{"ok": false, "error": "path does not exist: " + err.Error()})
		return
	}
	if !info.IsDir() {
		writeJSON(w, map[string]interface{}{"ok": false, "error": "path is not a directory"})
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true})
}

// SaveObsidian persists the vault path to .env and the running process.
// POST /api/integrations/obsidian/save  {vault_path: string}
func (h *Handler) SaveObsidian(w http.ResponseWriter, r *http.Request) {
	var req struct {
		VaultPath string `json:"vault_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	p := strings.TrimSpace(req.VaultPath)
	if p == "" {
		http.Error(w, "vault_path required", http.StatusBadRequest)
		return
	}
	if err := config.SetEnvAndPersist("VAULT_PATH", p); err != nil {
		internalError(w, "integrations.obsidian.save", err)
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true})
}

// --- Deepgram ---

// TestDeepgram creates a temporary client with the provided key and
// runs a health check against the Deepgram API. When api_key is empty,
// falls back to the currently-saved key on the registered client so
// the user can re-test an existing connection without re-entering it.
// POST /api/integrations/deepgram/test  {api_key?: string}
func (h *Handler) TestDeepgram(w http.ResponseWriter, r *http.Request) {
	var req struct {
		APIKey string `json:"api_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	key := strings.TrimSpace(req.APIKey)
	if key == "" {
		// Fall back to the saved key on the live integration so the
		// "Test" button keeps working after the user has saved.
		if dg := h.Registry.Get("deepgram"); dg != nil {
			if client, ok := dg.(*integrations.DeepgramClient); ok {
				key = strings.TrimSpace(client.APIKey)
			}
		}
	}
	if key == "" {
		writeJSON(w, map[string]interface{}{"ok": false, "error": "API key is required"})
		return
	}
	client := integrations.NewDeepgramClient(key)
	if err := client.HealthCheck(); err != nil {
		writeJSON(w, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true})
}

// SaveDeepgram persists the API key to .env and updates the running
// process environment. The registry's existing DeepgramClient reads
// from os.Getenv at call time so no re-registration is needed.
// POST /api/integrations/deepgram/save  {api_key: string}
func (h *Handler) SaveDeepgram(w http.ResponseWriter, r *http.Request) {
	var req struct {
		APIKey string `json:"api_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	key := strings.TrimSpace(req.APIKey)
	if key == "" {
		http.Error(w, "api_key required", http.StatusBadRequest)
		return
	}
	if err := config.SetEnvAndPersist("DEEPGRAM_API_KEY", key); err != nil {
		internalError(w, "integrations.deepgram.save", err)
		return
	}
	// Update the live client so the running process picks up the new key
	// without a restart.
	if dg := h.Registry.Get("deepgram"); dg != nil {
		if client, ok := dg.(*integrations.DeepgramClient); ok {
			client.APIKey = key
		}
	}
	writeJSON(w, map[string]interface{}{"ok": true})
}
