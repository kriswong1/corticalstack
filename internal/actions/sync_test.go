package actions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kriswong/corticalstack/internal/vault"
)

func TestSync_WritesToCentralFile(t *testing.T) {
	s := newTempStore(t)

	a, err := s.Upsert(&Action{
		Owner:       "Kris",
		Description: "Central only",
		Status:      StatusPending,
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := s.Sync(a); err != nil {
		t.Fatalf("sync: %v", err)
	}

	centralPath := filepath.Join(s.VaultPath(), s.CentralFilePath())
	data, err := os.ReadFile(centralPath)
	if err != nil {
		t.Fatalf("reading central file: %v", err)
	}
	if !strings.Contains(string(data), "id:"+a.ID) {
		t.Errorf("central file missing action; content:\n%s", data)
	}
}

func TestSync_WritesToSourceNote(t *testing.T) {
	s := newTempStore(t)

	a, err := s.Upsert(&Action{
		Owner:       "Kris",
		Description: "Note action",
		Status:      StatusDoing,
		SourceNote:  "notes/2026-04-11_hello.md",
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := s.Sync(a); err != nil {
		t.Fatalf("sync: %v", err)
	}

	notePath := filepath.Join(s.VaultPath(), a.SourceNote)
	data, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatalf("reading source note: %v", err)
	}
	if !strings.Contains(string(data), "id:"+a.ID) {
		t.Errorf("source note missing action; content:\n%s", data)
	}
	if !strings.Contains(string(data), "#status/doing") {
		t.Errorf("source note missing status tag; content:\n%s", data)
	}
}

func TestWriteOrReplaceLine_CreatesNewFile(t *testing.T) {
	dir := t.TempDir()
	v := vault.New(dir)
	relPath := "new-file.md"
	id := "aaaa-bbbb"
	line := "- [ ] [Kris] New task #status/pending <!-- id:" + id + " -->"

	if err := writeOrReplaceLine(v, relPath, id, line); err != nil {
		t.Fatalf("writeOrReplaceLine: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, relPath))
	if err != nil {
		t.Fatalf("reading new file: %v", err)
	}
	if !strings.Contains(string(data), line) {
		t.Errorf("file missing line; content:\n%s", data)
	}
}

func TestWriteOrReplaceLine_ReplacesExistingLine(t *testing.T) {
	dir := t.TempDir()
	v := vault.New(dir)
	relPath := "existing.md"
	id := "cccc-dddd"
	oldLine := "- [ ] [Kris] Old description #status/pending <!-- id:" + id + " -->"
	newLine := "- [x] [Kris] Old description #status/done <!-- id:" + id + " -->"

	initial := "# Tracker\n\n" + oldLine + "\n"
	if err := os.WriteFile(filepath.Join(dir, relPath), []byte(initial), 0o600); err != nil {
		t.Fatalf("writing seed file: %v", err)
	}

	if err := writeOrReplaceLine(v, relPath, id, newLine); err != nil {
		t.Fatalf("writeOrReplaceLine: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, relPath))
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	content := string(data)
	if strings.Contains(content, oldLine) {
		t.Errorf("old line still present; content:\n%s", content)
	}
	if !strings.Contains(content, newLine) {
		t.Errorf("new line missing; content:\n%s", content)
	}
}

func TestWriteOrReplaceLine_AppendsUnderHeader(t *testing.T) {
	dir := t.TempDir()
	v := vault.New(dir)
	relPath := "with-header.md"
	id := "eeee-ffff"
	line := "- [ ] [Kris] Under header #status/pending <!-- id:" + id + " -->"

	header := "# My Notes\n\nSome preamble.\n\n## Open Items\n\nExisting line.\n"
	if err := os.WriteFile(filepath.Join(dir, relPath), []byte(header), 0o600); err != nil {
		t.Fatalf("writing seed file: %v", err)
	}

	if err := writeOrReplaceLine(v, relPath, id, line); err != nil {
		t.Fatalf("writeOrReplaceLine: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, relPath))
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, line) {
		t.Errorf("line not found in file; content:\n%s", content)
	}
	// The new line should appear after "## Open Items", not at the very end.
	headerIdx := strings.Index(content, "## Open Items")
	lineIdx := strings.Index(content, line)
	if lineIdx < headerIdx {
		t.Errorf("line appeared before the header; content:\n%s", content)
	}
}
