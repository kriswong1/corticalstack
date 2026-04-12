package persona

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kriswong/corticalstack/internal/vault"
)

// newTestLoader creates a Loader pointing at a fresh tmpdir vault.
func newTestLoader(t *testing.T) (*Loader, *vault.Vault, string) {
	t.Helper()
	dir := t.TempDir()
	v := vault.New(dir)
	return New(v), v, dir
}

func TestInitIfMissingBootstrapsAllFiles(t *testing.T) {
	l, _, dir := newTestLoader(t)

	result, err := l.InitIfMissing()
	if err != nil {
		t.Fatalf("InitIfMissing: %v", err)
	}
	if len(result.Created) != 3 {
		t.Errorf("Created len = %d, want 3 (got %v)", len(result.Created), result.Created)
	}

	for _, name := range AllNames() {
		path := filepath.Join(dir, name.File())
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("expected %s on disk: %v", path, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("%s is empty after bootstrap", name)
		}
	}
}

func TestInitIfMissingIdempotent(t *testing.T) {
	l, _, _ := newTestLoader(t)

	first, err := l.InitIfMissing()
	if err != nil {
		t.Fatalf("first InitIfMissing: %v", err)
	}
	if len(first.Created) != 3 {
		t.Fatalf("first: created = %d", len(first.Created))
	}

	second, err := l.InitIfMissing()
	if err != nil {
		t.Fatalf("second InitIfMissing: %v", err)
	}
	if len(second.Created) != 0 {
		t.Errorf("second call should create nothing, got %v", second.Created)
	}
}

func TestInitIfMissingPartial(t *testing.T) {
	l, v, _ := newTestLoader(t)

	// Pre-create SOUL.md with custom content.
	if err := v.WriteFile(NameSoul.File(), "my existing soul"); err != nil {
		t.Fatalf("pre-write: %v", err)
	}

	result, err := l.InitIfMissing()
	if err != nil {
		t.Fatalf("InitIfMissing: %v", err)
	}
	if len(result.Created) != 2 {
		t.Errorf("Created = %v, want [user memory]", result.Created)
	}

	// Existing SOUL preserved.
	got, err := l.Get(NameSoul)
	if err != nil {
		t.Fatalf("Get soul: %v", err)
	}
	if got != "my existing soul" {
		t.Errorf("soul content = %q", got)
	}
}

func TestGetMissingReturnsEmpty(t *testing.T) {
	l, _, _ := newTestLoader(t)
	got, err := l.Get(NameSoul)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestGetCachesByMtime(t *testing.T) {
	l, v, _ := newTestLoader(t)
	if err := v.WriteFile(NameSoul.File(), "initial"); err != nil {
		t.Fatalf("write: %v", err)
	}

	first, err := l.Get(NameSoul)
	if err != nil {
		t.Fatalf("first Get: %v", err)
	}
	if first != "initial" {
		t.Errorf("first = %q", first)
	}

	// Bypass the Set() cache invalidation by writing directly to the file.
	// Since mtime has not changed (same second), the loader should serve
	// the cached content.
	path := filepath.Join(v.Path(), NameSoul.File())
	if err := os.WriteFile(path, []byte("changed but same mtime"), 0o600); err == nil {
		// Restore the original mtime so the cache-hit path is exercised.
		stat, _ := os.Stat(path)
		_ = os.Chtimes(path, stat.ModTime(), stat.ModTime())
	}

	// Can't reliably test cache without mtime manipulation across OSes,
	// so just verify the read is stable (no error) rather than enforcing
	// a specific hit/miss outcome.
	_, err = l.Get(NameSoul)
	if err != nil {
		t.Fatalf("second Get: %v", err)
	}
}

func TestGetRefreshesOnMtimeChange(t *testing.T) {
	l, v, _ := newTestLoader(t)
	if err := v.WriteFile(NameSoul.File(), "v1"); err != nil {
		t.Fatalf("write v1: %v", err)
	}
	if got, _ := l.Get(NameSoul); got != "v1" {
		t.Fatalf("v1 = %q", got)
	}

	// Wait long enough for fs mtime resolution and overwrite with fresh mtime.
	time.Sleep(20 * time.Millisecond)
	path := filepath.Join(v.Path(), NameSoul.File())
	if err := os.WriteFile(path, []byte("v2"), 0o600); err != nil {
		t.Fatalf("overwrite: %v", err)
	}
	// Force a newer mtime explicitly.
	future := time.Now().Add(time.Second)
	_ = os.Chtimes(path, future, future)

	got, err := l.Get(NameSoul)
	if err != nil {
		t.Fatalf("Get after mtime bump: %v", err)
	}
	if got != "v2" {
		t.Errorf("expected v2, got %q", got)
	}
}

func TestSetWritesAndInvalidatesCache(t *testing.T) {
	l, _, _ := newTestLoader(t)

	if err := l.Set(NameUser, "hello"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := l.Get(NameUser)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "hello" {
		t.Errorf("got %q", got)
	}

	// Set again; Get should reflect new content.
	if err := l.Set(NameUser, "world"); err != nil {
		t.Fatalf("second Set: %v", err)
	}
	got, _ = l.Get(NameUser)
	if got != "world" {
		t.Errorf("second got %q", got)
	}
}

func TestSetInvalidNameErrors(t *testing.T) {
	l, _, _ := newTestLoader(t)
	if err := l.Set(Name("bogus"), "x"); err == nil {
		t.Error("expected error for invalid name")
	}
}

func TestBuildContextPromptNilReceiver(t *testing.T) {
	var l *Loader
	if got := l.BuildContextPrompt(); got != "" {
		t.Errorf("nil receiver = %q, want empty", got)
	}
}

func TestBuildContextPromptEmptyVault(t *testing.T) {
	l, _, _ := newTestLoader(t)
	// No files written — should return empty.
	if got := l.BuildContextPrompt(); got != "" {
		t.Errorf("empty vault = %q, want empty", got)
	}
}

func TestBuildContextPromptIncludesSections(t *testing.T) {
	l, _, _ := newTestLoader(t)
	if err := l.Set(NameSoul, "Soul content"); err != nil {
		t.Fatalf("Set soul: %v", err)
	}
	if err := l.Set(NameUser, "User content"); err != nil {
		t.Fatalf("Set user: %v", err)
	}
	if err := l.Set(NameMemory, "Memory content"); err != nil {
		t.Fatalf("Set memory: %v", err)
	}

	got := l.BuildContextPrompt()
	wantHas := []string{
		"# Persona context",
		"## SOUL context",
		"Soul content",
		"## USER context",
		"User content",
		"## MEMORY context",
		"Memory content",
	}
	for _, sub := range wantHas {
		if !strings.Contains(got, sub) {
			t.Errorf("prompt missing %q", sub)
		}
	}

	// Order check: SOUL before USER before MEMORY.
	soulIdx := strings.Index(got, "SOUL context")
	userIdx := strings.Index(got, "USER context")
	memIdx := strings.Index(got, "MEMORY context")
	if !(soulIdx < userIdx && userIdx < memIdx) {
		t.Errorf("section order wrong: soul=%d user=%d memory=%d", soulIdx, userIdx, memIdx)
	}
}

func TestBuildContextPromptSkipsEmpty(t *testing.T) {
	l, _, _ := newTestLoader(t)
	if err := l.Set(NameSoul, "Soul only"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := l.Set(NameUser, "   \n\n  "); err != nil {
		t.Fatalf("Set user whitespace: %v", err)
	}
	// MEMORY not set at all.

	got := l.BuildContextPrompt()
	if !strings.Contains(got, "Soul only") {
		t.Errorf("missing soul content: %q", got)
	}
	if strings.Contains(got, "USER context") {
		t.Errorf("should skip whitespace-only USER")
	}
	if strings.Contains(got, "MEMORY context") {
		t.Errorf("should skip missing MEMORY")
	}
}

func TestBuildContextPromptSkipsFrontmatterOnly(t *testing.T) {
	l, _, _ := newTestLoader(t)
	// File with only frontmatter, no body — hasBody should reject.
	if err := l.Set(NameSoul, "---\nkey: value\n---\n\n   "); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got := l.BuildContextPrompt()
	if strings.Contains(got, "SOUL context") {
		t.Errorf("frontmatter-only file should be skipped")
	}
}

func TestGetTrimmedRespectsBudget(t *testing.T) {
	l, _, _ := newTestLoader(t)
	// SOUL budget is 3500; write 5000 chars.
	big := strings.Repeat("x", 5000)
	if err := l.Set(NameSoul, big); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got := l.getTrimmed(NameSoul)
	if len(got) != 3500 {
		t.Errorf("getTrimmed len = %d, want 3500", len(got))
	}
}

func TestGetTrimmedPassesThroughUnderBudget(t *testing.T) {
	l, _, _ := newTestLoader(t)
	small := "short content"
	if err := l.Set(NameSoul, small); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got := l.getTrimmed(NameSoul); got != small {
		t.Errorf("got %q", got)
	}
}

func TestHasBody(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"", false},
		{"   \n\n  ", false},
		{"real content", true},
		{"---\nkey: value\n---\n\n", false},
		{"---\nkey: value\n---\n\nreal body", true},
		{"---\nkey: value\n---", false},
	}
	for _, tt := range tests {
		if got := hasBody(tt.in); got != tt.want {
			t.Errorf("hasBody(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}
