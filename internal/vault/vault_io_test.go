package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewAndPath(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{"absolute path", "/tmp/vault"},
		{"relative path", "my-vault"},
		{"nested path", "/home/user/docs/vault"},
		{"empty string", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := New(tt.path)
			if v.Path() != tt.path {
				t.Errorf("Path() = %q, want %q", v.Path(), tt.path)
			}
		})
	}
}

func TestWriteNoteReadNoteRoundtrip(t *testing.T) {
	v := New(t.TempDir())

	note := &Note{
		Frontmatter: map[string]interface{}{
			"title": "Test Note",
			"type":  "meeting",
			"tags":  []interface{}{"go", "testing"},
		},
		Body: "# Test Note\n\nSome body content.",
	}

	relPath := "notes/test-note.md"
	if err := v.WriteNote(relPath, note); err != nil {
		t.Fatalf("WriteNote: %v", err)
	}

	got, err := v.ReadNote(relPath)
	if err != nil {
		t.Fatalf("ReadNote: %v", err)
	}

	if got.Path != relPath {
		t.Errorf("Path = %q, want %q", got.Path, relPath)
	}
	if got.Body != note.Body {
		t.Errorf("Body = %q, want %q", got.Body, note.Body)
	}
	for _, key := range []string{"title", "type", "tags"} {
		if _, ok := got.Frontmatter[key]; !ok {
			t.Errorf("frontmatter missing key %q", key)
		}
	}
	if got.Frontmatter["title"] != "Test Note" {
		t.Errorf("title = %v, want %q", got.Frontmatter["title"], "Test Note")
	}
}

func TestWriteFileReadFileRoundtrip(t *testing.T) {
	v := New(t.TempDir())

	tests := []struct {
		name    string
		relPath string
		content string
	}{
		{"plain text", "file.txt", "hello world"},
		{"markdown", "docs/readme.md", "# Title\n\nParagraph.\n"},
		{"empty content", "empty.txt", ""},
		{"unicode", "uni.txt", "emoji 🌍 and 日本語"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := v.WriteFile(tt.relPath, tt.content); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}
			got, err := v.ReadFile(tt.relPath)
			if err != nil {
				t.Fatalf("ReadFile: %v", err)
			}
			if got != tt.content {
				t.Errorf("got %q, want %q", got, tt.content)
			}
		})
	}
}

func TestReadNoteNonexistent(t *testing.T) {
	v := New(t.TempDir())
	_, err := v.ReadNote("does/not/exist.md")
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}
}

func TestReadFileNonexistent(t *testing.T) {
	v := New(t.TempDir())
	_, err := v.ReadFile("nope.txt")
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}
}

func TestExists(t *testing.T) {
	v := New(t.TempDir())

	if v.Exists("missing.md") {
		t.Error("Exists returned true for missing file")
	}

	if err := v.WriteFile("present.md", "hi"); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if !v.Exists("present.md") {
		t.Error("Exists returned false for present file")
	}
}

func TestWriteNoteCreatesIntermediateDirectories(t *testing.T) {
	v := New(t.TempDir())

	relPath := "a/b/c/deep-note.md"
	note := &Note{
		Frontmatter: map[string]interface{}{"title": "Deep"},
		Body:        "nested content\n",
	}

	if err := v.WriteNote(relPath, note); err != nil {
		t.Fatalf("WriteNote: %v", err)
	}

	fullPath := filepath.Join(v.Path(), relPath)
	if _, err := os.Stat(fullPath); err != nil {
		t.Errorf("file not created at %s: %v", fullPath, err)
	}
}

func TestEnsureDailyLog(t *testing.T) {
	v := New(t.TempDir())

	if err := v.EnsureDailyLog(); err != nil {
		t.Fatalf("first EnsureDailyLog: %v", err)
	}

	relPath := TodayLogPath()
	if !v.Exists(relPath) {
		t.Fatal("daily log not created")
	}

	content1, err := v.ReadFile(relPath)
	if err != nil {
		t.Fatalf("ReadFile after first call: %v", err)
	}

	// Idempotent: second call should not error or change content.
	if err := v.EnsureDailyLog(); err != nil {
		t.Fatalf("second EnsureDailyLog: %v", err)
	}

	content2, err := v.ReadFile(relPath)
	if err != nil {
		t.Fatalf("ReadFile after second call: %v", err)
	}
	if content1 != content2 {
		t.Error("EnsureDailyLog modified existing file on second call")
	}
}

func TestAppendToDaily(t *testing.T) {
	v := New(t.TempDir())

	if err := v.AppendToDaily("first entry"); err != nil {
		t.Fatalf("AppendToDaily: %v", err)
	}
	if err := v.AppendToDaily("second entry"); err != nil {
		t.Fatalf("AppendToDaily second: %v", err)
	}

	content, err := v.ReadFile(TodayLogPath())
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	if !strings.Contains(content, "first entry") {
		t.Error("daily log missing 'first entry'")
	}
	if !strings.Contains(content, "second entry") {
		t.Error("daily log missing 'second entry'")
	}
	// Entries should be timestamped bullet points.
	if !strings.Contains(content, "- **") {
		t.Error("daily log missing timestamped bullet format")
	}
}

func TestAppendActionItems(t *testing.T) {
	v := New(t.TempDir())

	items := []ActionItem{
		{Owner: "Alice", Description: "Review PR", Deadline: "2026-04-15"},
		{Owner: "", Description: "Update docs"},
		{Owner: "Bob", Description: "Deploy staging", Deadline: ""},
	}

	if err := v.AppendActionItems("Sprint Review", "2026-04-11", items); err != nil {
		t.Fatalf("AppendActionItems: %v", err)
	}

	content, err := v.ReadFile(actionItemsFile)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	checks := []string{
		"# Action Items",
		"## Open Items",
		"### From: Sprint Review (2026-04-11)",
		"- [ ] [Alice] Review PR *(due: 2026-04-15)*",
		"- [ ] [TBD] Update docs",
		"- [ ] [Bob] Deploy staging",
	}
	for _, want := range checks {
		if !strings.Contains(content, want) {
			t.Errorf("missing %q in:\n%s", want, content)
		}
	}

	// Bob's entry should not have a deadline suffix.
	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(line, "[Bob]") && strings.Contains(line, "*(due:") {
			t.Error("Bob's item should not have a deadline")
		}
	}

	// Append a second batch to verify insertion order.
	items2 := []ActionItem{
		{Owner: "Carol", Description: "Fix bug"},
	}
	if err := v.AppendActionItems("Hotfix", "2026-04-12", items2); err != nil {
		t.Fatalf("second AppendActionItems: %v", err)
	}

	content2, err := v.ReadFile(actionItemsFile)
	if err != nil {
		t.Fatalf("ReadFile second: %v", err)
	}

	if !strings.Contains(content2, "### From: Hotfix (2026-04-12)") {
		t.Error("second batch heading missing")
	}

	// Newest batch should appear before the first batch.
	idxHotfix := strings.Index(content2, "Hotfix")
	idxSprint := strings.Index(content2, "Sprint Review")
	if idxHotfix > idxSprint {
		t.Error("newer items should appear before older items under Open Items")
	}
}
