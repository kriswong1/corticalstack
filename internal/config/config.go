// Package config loads environment variables and exposes typed accessors.
package config

import (
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/joho/godotenv"

	"github.com/kriswong/corticalstack/internal/platform"
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
// When running under WSL2, a Windows drive-letter value (e.g.
// `C:\Users\kris\vault`) is auto-translated to its `/mnt/c/...` equivalent
// so users can share the same .env between native Windows and WSL2.
func VaultPath() string {
	Load()
	if v := os.Getenv("VAULT_PATH"); v != "" {
		return platform.MaybeTranslateForWSL(v)
	}
	return "vault"
}

// Port returns the HTTP server port, defaulting to 8000.
// Values outside the valid TCP range (1-65535) are ignored.
func Port() int {
	Load()
	if v := os.Getenv("PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= 65535 {
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

// UsageLogPath returns the path to the JSONL file where every Claude
// CLI invocation is recorded. Default: <VAULT_PATH>/.cortical/usage.jsonl
// (the dot-prefix keeps Obsidian from indexing it). Override with
// USAGE_LOG_PATH env var.
func UsageLogPath() string {
	Load()
	if v := os.Getenv("USAGE_LOG_PATH"); v != "" {
		return platform.MaybeTranslateForWSL(v)
	}
	return filepath.Join(VaultPath(), ".cortical", "usage.jsonl")
}

// ItemUsageLogPath returns the path to the JSONL file where item-
// tagged Claude CLI invocations are recorded. Default:
// <VAULT_PATH>/.cortical/item-usage.jsonl — sibling of usage.jsonl,
// dot-prefixed parent so Obsidian doesn't index it. Override with
// ITEM_USAGE_LOG_PATH env var.
//
// Kept distinct from UsageLogPath so the two indices can evolve
// independently and so an env-var override can point one at a
// different disk than the other (e.g. SSD vs spinning).
func ItemUsageLogPath() string {
	Load()
	if v := os.Getenv("ITEM_USAGE_LOG_PATH"); v != "" {
		return platform.MaybeTranslateForWSL(v)
	}
	return filepath.Join(VaultPath(), ".cortical", "item-usage.jsonl")
}

// DeepgramAPIKey returns the Deepgram API key from the environment.
func DeepgramAPIKey() string {
	Load()
	return os.Getenv("DEEPGRAM_API_KEY")
}

// MaxUploadBytes returns the upload size cap, defaulting to 200 MB.
func MaxUploadBytes() int64 {
	Load()
	if v := os.Getenv("MAX_UPLOAD_BYTES"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			return n
		}
	}
	return 200 << 20
}

// MaxExtractionChars returns the extraction prompt content cap, defaulting to 50000.
func MaxExtractionChars() int {
	Load()
	if v := os.Getenv("MAX_EXTRACTION_CHARS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 50_000
}

// MaxClassifierChars returns the classifier prompt content cap, defaulting to 8000.
func MaxClassifierChars() int {
	Load()
	if v := os.Getenv("MAX_CLASSIFIER_CHARS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 8000
}

// WIPLimit returns the max number of actions allowed in "doing" status,
// defaulting to 3. Set WIP_LIMIT=0 to disable.
func WIPLimit() int {
	Load()
	if v := os.Getenv("WIP_LIMIT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return n
		}
	}
	return 3
}

// GetSecret reads an arbitrary environment variable.
// All API keys should go through this helper so future runtime secret
// stores can be added without touching callers.
func GetSecret(key string) string {
	Load()
	return os.Getenv(key)
}
