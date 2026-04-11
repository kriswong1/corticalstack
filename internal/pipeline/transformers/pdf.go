package transformers

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/kriswong/corticalstack/internal/pipeline"
	"github.com/ledongthuc/pdf"
)

// PDFTransformer extracts text from PDF files using pure-Go library.
type PDFTransformer struct{}

func (t *PDFTransformer) Name() string { return "pdf" }

func (t *PDFTransformer) CanHandle(input *pipeline.RawInput) bool {
	ext := strings.ToLower(filepath.Ext(input.Path))
	if ext == "" {
		ext = strings.ToLower(filepath.Ext(input.Filename))
	}
	return ext == ".pdf"
}

func (t *PDFTransformer) Transform(input *pipeline.RawInput) (*pipeline.TextDocument, error) {
	text, err := extractPDFText(input)
	if err != nil {
		return nil, fmt.Errorf("extracting pdf text: %w", err)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("pdf had no extractable text (may be scanned images)")
	}

	title := input.Title
	if title == "" {
		name := input.Filename
		if name == "" {
			name = filepath.Base(input.Path)
		}
		title = strings.TrimSuffix(name, filepath.Ext(name))
	}

	return &pipeline.TextDocument{
		ID:      identifierFor(input),
		Source:  "pdf",
		Title:   title,
		Date:    fileModTime(input.Path),
		Content: text,
		Metadata: mergeMeta(input.Metadata, map[string]string{
			"input_file": input.Path,
		}),
	}, nil
}

func extractPDFText(input *pipeline.RawInput) (string, error) {
	var reader *pdf.Reader
	var err error

	if len(input.Content) > 0 {
		rs := bytes.NewReader(input.Content)
		reader, err = pdf.NewReader(rs, int64(len(input.Content)))
		if err != nil {
			return "", err
		}
	} else if input.Path != "" {
		f, r, err := pdf.Open(input.Path)
		if err != nil {
			return "", err
		}
		defer f.Close()
		reader = r
	} else {
		return "", fmt.Errorf("no pdf path or content provided")
	}

	var buf strings.Builder
	totalPages := reader.NumPage()
	for i := 1; i <= totalPages; i++ {
		page := reader.Page(i)
		if page.V.IsNull() {
			continue
		}
		text, err := page.GetPlainText(nil)
		if err != nil {
			continue
		}
		buf.WriteString(text)
		buf.WriteString("\n\n")
	}
	return buf.String(), nil
}
