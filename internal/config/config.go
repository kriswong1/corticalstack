// Package config loads environment variables and exposes typed accessors.
package config

import (
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/joho/godotenv"
)

var once sync.Once

// Load reads .env from the current directory, then falls back to the
// executable's directory. Safe to call multiple times.
func Load() {
	once.Do(func() {
		if err := godotenv.Load(); err != nil {
			if ex, err := os.Executable(); err == nil {
				_ = godotenv.Load(filepath.Join(filepath.Dir(ex), ".env"))
			}
		}
	})
}

// VaultPath returns the configured Obsidian vault path, defaulting to ./vault.
func VaultPath() string {
	Load()
	if v := os.Getenv("VAULT_PATH"); v != "" {
		return v
	}
	return "vault"
}

// Port returns the HTTP server port, defaulting to 8000.
func Port() int {
	Load()
	if v := os.Getenv("PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return 8000
}

// ClaudeModel returns the preferred Claude CLI model (empty string = CLI default).
func ClaudeModel() string {
	Load()
	return os.Getenv("CLAUDE_MODEL")
}

// DeepgramAPIKey returns the Deepgram API key from the environment.
func DeepgramAPIKey() string {
	Load()
	return os.Getenv("DEEPGRAM_API_KEY")
}

// GetSecret reads an arbitrary environment variable.
// All API keys should go through this helper so future runtime secret
// stores can be added without touching callers.
func GetSecret(key string) string {
	Load()
	return os.Getenv(key)
}
