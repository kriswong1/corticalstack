package transformers

import (
	"testing"
)

func TestNewDefaultReturnsNonEmpty(t *testing.T) {
	transformers := NewDefault(nil, nil)
	if len(transformers) == 0 {
		t.Fatal("NewDefault(nil, nil) returned empty slice")
	}
}

func TestNewDefaultAllHaveNames(t *testing.T) {
	transformers := NewDefault(nil, nil)
	for i, tr := range transformers {
		name := tr.Name()
		if name == "" {
			t.Errorf("transformer at index %d has empty Name()", i)
		}
	}
}

func TestNewDefaultExpectedTransformers(t *testing.T) {
	transformers := NewDefault(nil, nil)
	names := make(map[string]bool)
	for _, tr := range transformers {
		names[tr.Name()] = true
	}

	expected := []string{"passthrough", "youtube", "linkedin", "webpage", "html", "pdf", "docx", "deepgram", "vtt"}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("expected transformer %q not found in NewDefault(nil, nil); got %v", name, names)
		}
	}
}

func TestNewDefaultPassthroughIsLast(t *testing.T) {
	transformers := NewDefault(nil, nil)
	last := transformers[len(transformers)-1]
	if last.Name() != "passthrough" {
		t.Errorf("last transformer = %q, want %q (passthrough should be the fallback)", last.Name(), "passthrough")
	}
}

func TestNewDefaultNoDuplicateNames(t *testing.T) {
	transformers := NewDefault(nil, nil)
	seen := make(map[string]int)
	for i, tr := range transformers {
		name := tr.Name()
		if prev, ok := seen[name]; ok {
			t.Errorf("duplicate transformer name %q at indices %d and %d", name, prev, i)
		}
		seen[name] = i
	}
}
