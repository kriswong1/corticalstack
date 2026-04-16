package platform

import "testing"

func TestTranslateWindowsPath(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"backslash drive letter", `C:\Users\kris\vault`, "/mnt/c/Users/kris/vault"},
		{"forward slash drive letter", "C:/Users/kris/vault", "/mnt/c/Users/kris/vault"},
		{"lowercase drive letter", `d:\data\notes`, "/mnt/d/data/notes"},
		{"drive root backslash", `C:\`, "/mnt/c/"},
		{"drive root forward slash", "C:/", "/mnt/c/"},
		{"mixed separators", `C:\Users/kris\vault`, "/mnt/c/Users/kris/vault"},
		{"already linux", "/home/kris/vault", "/home/kris/vault"},
		{"already mnt path", "/mnt/c/Users/kris", "/mnt/c/Users/kris"},
		{"relative", "vault", "vault"},
		{"relative with dot", "./vault", "./vault"},
		{"empty", "", ""},
		{"drive-relative not touched", "C:foo", "C:foo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TranslateWindowsPath(tt.in); got != tt.want {
				t.Errorf("TranslateWindowsPath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestDetectWSL(t *testing.T) {
	// Isolate from any ambient WSL hints on the host running this test.
	t.Setenv("CORTICAL_WSL", "")
	t.Setenv("WSL_DISTRO_NAME", "")
	t.Setenv("WSL_INTEROP", "")

	t.Run("explicit override", func(t *testing.T) {
		t.Setenv("CORTICAL_WSL", "1")
		if !detectWSL() {
			t.Error("expected detectWSL true with CORTICAL_WSL=1")
		}
	})

	t.Run("distro name", func(t *testing.T) {
		t.Setenv("CORTICAL_WSL", "")
		t.Setenv("WSL_DISTRO_NAME", "Ubuntu")
		if !detectWSL() {
			t.Error("expected detectWSL true with WSL_DISTRO_NAME set")
		}
	})

	t.Run("interop", func(t *testing.T) {
		t.Setenv("CORTICAL_WSL", "")
		t.Setenv("WSL_DISTRO_NAME", "")
		t.Setenv("WSL_INTEROP", "/run/WSL/1_interop")
		if !detectWSL() {
			t.Error("expected detectWSL true with WSL_INTEROP set")
		}
	})
}
