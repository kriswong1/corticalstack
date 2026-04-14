package vault

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSafeRelPath covers CR-01: every user-supplied vault path must be
// normalized and rejected if it tries to escape the vault root.
func TestSafeRelPath(t *testing.T) {
	v := New(t.TempDir())

	t.Run("accepts nested legit path", func(t *testing.T) {
		got, err := v.SafeRelPath("product/pitch/foo.md")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Clean converts forward slashes to OS separator; verify both
		// forms map back to the expected cleaned form.
		want := filepath.Clean("product/pitch/foo.md")
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("accepts file at root", func(t *testing.T) {
		got, err := v.SafeRelPath("notes.md")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "notes.md" {
			t.Errorf("got %q, want %q", got, "notes.md")
		}
	})

	t.Run("accepts leading dot segment", func(t *testing.T) {
		// ./foo.md should Clean to foo.md and be accepted.
		got, err := v.SafeRelPath("./foo.md")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "foo.md" {
			t.Errorf("got %q, want %q", got, "foo.md")
		}
	})

	t.Run("rejects empty path", func(t *testing.T) {
		_, err := v.SafeRelPath("")
		if err == nil {
			t.Fatal("expected error for empty path, got nil")
		}
		if !errors.Is(err, ErrUnsafePath) {
			t.Errorf("want ErrUnsafePath, got %v", err)
		}
	})

	t.Run("rejects null byte", func(t *testing.T) {
		_, err := v.SafeRelPath("foo\x00bar.md")
		if err == nil {
			t.Fatal("expected error for null byte, got nil")
		}
		if !errors.Is(err, ErrUnsafePath) {
			t.Errorf("want ErrUnsafePath, got %v", err)
		}
	})

	t.Run("rejects absolute unix path", func(t *testing.T) {
		_, err := v.SafeRelPath("/etc/passwd")
		if err == nil {
			t.Fatal("expected error for absolute path, got nil")
		}
		if !errors.Is(err, ErrUnsafePath) {
			t.Errorf("want ErrUnsafePath, got %v", err)
		}
	})

	t.Run("rejects absolute windows path", func(t *testing.T) {
		_, err := v.SafeRelPath(`C:\Windows\System32\config\SAM`)
		if err == nil {
			t.Fatal("expected error for windows absolute path, got nil")
		}
		if !errors.Is(err, ErrUnsafePath) {
			t.Errorf("want ErrUnsafePath, got %v", err)
		}
	})

	t.Run("rejects volume-relative windows path", func(t *testing.T) {
		// On Windows, "C:foo" is not IsAbs but still escapes the vault
		// by targeting the current directory of drive C:. Reject it.
		_, err := v.SafeRelPath("C:foo")
		if err == nil {
			t.Fatal("expected error for volume-relative path, got nil")
		}
		if !errors.Is(err, ErrUnsafePath) {
			t.Errorf("want ErrUnsafePath, got %v", err)
		}
	})

	t.Run("rejects parent traversal", func(t *testing.T) {
		dangerous := []string{
			"../etc/passwd",
			"../../etc/passwd",
			"../../../etc/passwd",
			"../../../../Users/me/.ssh/id_rsa",
			"..",
		}
		for _, p := range dangerous {
			t.Run(p, func(t *testing.T) {
				_, err := v.SafeRelPath(p)
				if err == nil {
					t.Fatalf("path %q was NOT rejected", p)
				}
				if !errors.Is(err, ErrUnsafePath) {
					t.Errorf("want ErrUnsafePath, got %v", err)
				}
			})
		}
	})

	t.Run("rejects nested traversal", func(t *testing.T) {
		// filepath.Clean collapses these so the ".." climbs above the
		// vault root.
		dangerous := []string{
			"notes/../../etc/passwd",
			"foo/../../bar",
			"a/b/c/../../../../etc/passwd",
		}
		for _, p := range dangerous {
			t.Run(p, func(t *testing.T) {
				_, err := v.SafeRelPath(p)
				if err == nil {
					t.Fatalf("path %q was NOT rejected", p)
				}
				if !errors.Is(err, ErrUnsafePath) {
					t.Errorf("want ErrUnsafePath, got %v", err)
				}
			})
		}
	})

	t.Run("accepts path with internal dot-dot that stays within", func(t *testing.T) {
		// "a/b/../c.md" → "a/c.md", still inside the vault. Must NOT
		// be rejected (only traversal that exits the vault is unsafe).
		got, err := v.SafeRelPath("a/b/../c.md")
		if err != nil {
			t.Fatalf("unexpected error for safe inner .. : %v", err)
		}
		want := filepath.Clean("a/c.md")
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}

// TestReadFileTraversalBlocked verifies that ReadFile itself now rejects
// traversal paths even if a caller forgot to validate. This is the defense-
// in-depth layer behind the handler-boundary check.
func TestReadFileTraversalBlocked(t *testing.T) {
	dir := t.TempDir()

	// Write a secret next to the vault so traversal would be required
	// to reach it.
	parent := filepath.Dir(dir)
	secret := filepath.Join(parent, "SECRET.txt")
	if err := os.WriteFile(secret, []byte("top secret"), 0o600); err != nil {
		t.Fatalf("seed secret: %v", err)
	}
	defer os.Remove(secret)

	v := New(dir)

	// Try to read the sibling secret via traversal.
	_, err := v.ReadFile("../" + filepath.Base(secret))
	if err == nil {
		t.Fatal("ReadFile was NOT blocked — path traversal succeeded")
	}
	if !errors.Is(err, ErrUnsafePath) {
		t.Errorf("want ErrUnsafePath, got %v", err)
	}
}

// TestWriteFileTraversalBlocked ensures a traversal write can't escape
// the vault either.
func TestWriteFileTraversalBlocked(t *testing.T) {
	dir := t.TempDir()
	v := New(dir)

	err := v.WriteFile("../escaped.txt", "should not be written")
	if err == nil {
		t.Fatal("WriteFile was NOT blocked — traversal write succeeded")
	}
	if !errors.Is(err, ErrUnsafePath) {
		t.Errorf("want ErrUnsafePath, got %v", err)
	}
	// Double-check nothing was actually written outside the vault.
	parent := filepath.Dir(dir)
	escaped := filepath.Join(parent, "escaped.txt")
	if _, err := os.Stat(escaped); !os.IsNotExist(err) {
		// Clean up just in case some future regression leaks.
		_ = os.Remove(escaped)
		t.Errorf("escaped file was created at %s", escaped)
	}

	// Sanity: legitimate write still works and the returned content
	// includes the vault-local path.
	if err := v.WriteFile("ok.md", "hi"); err != nil {
		t.Fatalf("legitimate WriteFile failed: %v", err)
	}
	got, err := v.ReadFile("ok.md")
	if err != nil {
		t.Fatalf("legitimate ReadFile failed: %v", err)
	}
	if !strings.Contains(got, "hi") {
		t.Errorf("got %q, want to contain 'hi'", got)
	}
}

func TestSlugify(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"Hello World", "hello-world"},
		{"Already-Slugified-123", "already-slugified-123"},
		{"MixedCASE", "mixedcase"},
		{"  leading and trailing  ", "leading-and-trailing"},
		{"multiple   spaces", "multiple-spaces"},
		{"under_score_test", "under-score-test"},
		{"punctuation!!! matters???", "punctuation-matters"},
		{"emoji 🌍 test", "emoji-test"},
		{"日本語", ""},
		{"", ""},
		{"---", ""},
		{"dash--in--middle", "dash-in-middle"},
		{"a1b2c3", "a1b2c3"},
		{"   ", ""},
		{"hello_world foo-bar BAZ", "hello-world-foo-bar-baz"},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got := Slugify(tt.in)
			if got != tt.want {
				t.Errorf("Slugify(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
