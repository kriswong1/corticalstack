package transformers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kriswong/corticalstack/internal/pipeline"
)

func TestPassthroughName(t *testing.T) {
	tr := &PassthroughTransformer{}
	if got := tr.Name(); got != "passthrough" {
		t.Errorf("Name() = %q, want %q", got, "passthrough")
	}
}

func TestPassthroughCanHandle(t *testing.T) {
	tr := &PassthroughTransformer{}
	tests := []struct {
		name  string
		input pipeline.RawInput
		want  bool
	}{
		{
			name:  "InputText always handled",
			input: pipeline.RawInput{Kind: pipeline.InputText, Content: []byte("hello")},
			want:  true,
		},
		{
			name:  "InputFile with no extension",
			input: pipeline.RawInput{Kind: pipeline.InputFile, Filename: "notes"},
			want:  true,
		},
		{
			name:  "InputFile with .txt extension",
			input: pipeline.RawInput{Kind: pipeline.InputFile, Filename: "readme.txt"},
			want:  true,
		},
		{
			name:  "InputFile with .md extension",
			input: pipeline.RawInput{Kind: pipeline.InputFile, Filename: "README.md"},
			want:  true,
		},
		{
			name:  "InputFile with .markdown extension",
			input: pipeline.RawInput{Kind: pipeline.InputFile, Filename: "doc.markdown"},
			want:  true,
		},
		{
			name:  "InputFile with .pdf extension rejected",
			input: pipeline.RawInput{Kind: pipeline.InputFile, Filename: "doc.pdf"},
			want:  false,
		},
		{
			name:  "InputFile with .html extension rejected",
			input: pipeline.RawInput{Kind: pipeline.InputFile, Filename: "page.html"},
			want:  false,
		},
		{
			name:  "InputURL with no path or filename accepted (empty ext matches fallback)",
			input: pipeline.RawInput{Kind: pipeline.InputURL, URL: "https://example.com"},
			want:  true,
		},
		{
			name:  "InputFile path used when filename empty",
			input: pipeline.RawInput{Kind: pipeline.InputFile, Path: "/tmp/data.txt"},
			want:  true,
		},
		{
			name:  "InputFile path with unknown ext rejected",
			input: pipeline.RawInput{Kind: pipeline.InputFile, Path: "/tmp/data.docx"},
			want:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tr.CanHandle(&tt.input)
			if got != tt.want {
				t.Errorf("CanHandle() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPassthroughTransformInputText(t *testing.T) {
	tr := &PassthroughTransformer{}
	input := &pipeline.RawInput{
		Kind:    pipeline.InputText,
		Content: []byte("Hello, world!"),
	}

	doc, err := tr.Transform(input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if doc.Source != "passthrough" {
		t.Errorf("Source = %q, want %q", doc.Source, "passthrough")
	}
	if doc.Content != "Hello, world!" {
		t.Errorf("Content = %q, want %q", doc.Content, "Hello, world!")
	}
	if doc.Metadata["input_kind"] != "text" {
		t.Errorf("Metadata[input_kind] = %q, want %q", doc.Metadata["input_kind"], "text")
	}
}

func TestPassthroughTransformInputTextWithTitle(t *testing.T) {
	tr := &PassthroughTransformer{}
	input := &pipeline.RawInput{
		Kind:    pipeline.InputText,
		Content: []byte("Some content here"),
		Title:   "My Custom Title",
	}

	doc, err := tr.Transform(input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if doc.Title != "My Custom Title" {
		t.Errorf("Title = %q, want %q", doc.Title, "My Custom Title")
	}
}

func TestPassthroughTransformInputTextTitleFromFirstLine(t *testing.T) {
	tr := &PassthroughTransformer{}
	input := &pipeline.RawInput{
		Kind:    pipeline.InputText,
		Content: []byte("First line title\nSecond line body"),
	}

	doc, err := tr.Transform(input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if doc.Title != "First line title" {
		t.Errorf("Title = %q, want %q", doc.Title, "First line title")
	}
}

func TestPassthroughTransformInputFile(t *testing.T) {
	tr := &PassthroughTransformer{}

	dir := t.TempDir()
	path := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(path, []byte("File content here"), 0644); err != nil {
		t.Fatalf("writing temp file: %v", err)
	}

	input := &pipeline.RawInput{
		Kind:     pipeline.InputFile,
		Path:     path,
		Filename: "notes.txt",
	}

	doc, err := tr.Transform(input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if doc.Content != "File content here" {
		t.Errorf("Content = %q, want %q", doc.Content, "File content here")
	}
	if doc.Source != "passthrough" {
		t.Errorf("Source = %q, want %q", doc.Source, "passthrough")
	}
	// Title should be filename stem when no Title provided
	if doc.Title != "notes" {
		t.Errorf("Title = %q, want %q", doc.Title, "notes")
	}
	if doc.Metadata["input_kind"] != "file" {
		t.Errorf("Metadata[input_kind] = %q, want %q", doc.Metadata["input_kind"], "file")
	}
}

func TestPassthroughTransformInputFilePath(t *testing.T) {
	tr := &PassthroughTransformer{}

	dir := t.TempDir()
	path := filepath.Join(dir, "journal.md")
	if err := os.WriteFile(path, []byte("# Journal\nToday was good."), 0644); err != nil {
		t.Fatalf("writing temp file: %v", err)
	}

	// No Filename set, only Path — title should come from Path base
	input := &pipeline.RawInput{
		Kind: pipeline.InputFile,
		Path: path,
	}

	doc, err := tr.Transform(input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if doc.Title != "journal" {
		t.Errorf("Title = %q, want %q", doc.Title, "journal")
	}
}

func TestPassthroughTransformEmptyContent(t *testing.T) {
	tr := &PassthroughTransformer{}
	input := &pipeline.RawInput{
		Kind:    pipeline.InputText,
		Content: []byte(""),
	}

	_, err := tr.Transform(input)
	if err == nil {
		t.Error("Transform() expected error for empty input, got nil")
	}
}

func TestFirstLineTitle(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "simple first line",
			content: "Hello World\nMore content",
			want:    "Hello World",
		},
		{
			name:    "skip leading blank lines",
			content: "\n\n  \nActual Title\nBody",
			want:    "Actual Title",
		},
		{
			name:    "strip markdown heading",
			content: "# Heading\nBody text",
			want:    "Heading",
		},
		{
			name:    "strip multiple hash marks",
			content: "## Sub Heading\nContent",
			want:    "Sub Heading",
		},
		{
			name:    "truncate to 80 chars",
			content: strings.Repeat("x", 100) + "\nShort line",
			want:    strings.Repeat("x", 80),
		},
		{
			name:    "exactly 80 chars not truncated",
			content: strings.Repeat("a", 80) + "\nOther",
			want:    strings.Repeat("a", 80),
		},
		{
			name:    "empty content returns Untitled",
			content: "",
			want:    "Untitled",
		},
		{
			name:    "only blank lines returns Untitled",
			content: "\n  \n\t\n",
			want:    "Untitled",
		},
		{
			name:    "single line no newline",
			content: "Just one line",
			want:    "Just one line",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := firstLineTitle(tt.content)
			if got != tt.want {
				t.Errorf("firstLineTitle() = %q, want %q", got, tt.want)
			}
		})
	}
}
