package config

import "testing"

func TestVaultPath(t *testing.T) {
	t.Run("default when unset", func(t *testing.T) {
		t.Setenv("VAULT_PATH", "")
		if got := VaultPath(); got != "vault" {
			t.Errorf("VaultPath() = %q, want %q", got, "vault")
		}
	})

	t.Run("env override", func(t *testing.T) {
		t.Setenv("VAULT_PATH", "/custom/path")
		if got := VaultPath(); got != "/custom/path" {
			t.Errorf("VaultPath() = %q, want %q", got, "/custom/path")
		}
	})
}

func TestPort(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want int
	}{
		{"default when unset", "", 8000},
		{"numeric override", "3000", 3000},
		{"non-numeric falls back", "notanumber", 8000},
		{"whitespace is non-numeric", "  ", 8000},
		{"negative number accepted as-is", "-1", -1},
		{"zero accepted as-is", "0", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PORT", tt.env)
			if got := Port(); got != tt.want {
				t.Errorf("Port() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestClaudeModel(t *testing.T) {
	t.Run("empty when unset", func(t *testing.T) {
		t.Setenv("CLAUDE_MODEL", "")
		if got := ClaudeModel(); got != "" {
			t.Errorf("ClaudeModel() = %q, want empty", got)
		}
	})

	t.Run("env override", func(t *testing.T) {
		t.Setenv("CLAUDE_MODEL", "claude-sonnet-4-6")
		if got := ClaudeModel(); got != "claude-sonnet-4-6" {
			t.Errorf("ClaudeModel() = %q", got)
		}
	})
}

func TestDeepgramAPIKey(t *testing.T) {
	t.Run("empty when unset", func(t *testing.T) {
		t.Setenv("DEEPGRAM_API_KEY", "")
		if got := DeepgramAPIKey(); got != "" {
			t.Errorf("DeepgramAPIKey() = %q, want empty", got)
		}
	})

	t.Run("env override", func(t *testing.T) {
		t.Setenv("DEEPGRAM_API_KEY", "test-key-abc123")
		if got := DeepgramAPIKey(); got != "test-key-abc123" {
			t.Errorf("DeepgramAPIKey() = %q", got)
		}
	})
}

func TestGetSecret(t *testing.T) {
	t.Setenv("MY_SECRET_VAR", "secret-value")
	if got := GetSecret("MY_SECRET_VAR"); got != "secret-value" {
		t.Errorf("GetSecret = %q, want %q", got, "secret-value")
	}
	if got := GetSecret("UNSET_SECRET_VAR_XYZ"); got != "" {
		t.Errorf("GetSecret unset = %q, want empty", got)
	}
}
