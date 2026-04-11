package transformers

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/kriswong/corticalstack/internal/pipeline"
)

// DOCXTransformer extracts text from .docx files by reading word/document.xml
// directly from the zip container. No Python or pandoc required.
type DOCXTransformer struct{}

func (t *DOCXTransformer) Name() string { return "docx" }

func (t *DOCXTransformer) CanHandle(input *pipeline.RawInput) bool {
	ext := strings.ToLower(filepath.Ext(input.Path))
	if ext == "" {
		ext = strings.ToLower(filepath.Ext(input.Filename))
	}
	return ext == ".docx"
}

func (t *DOCXTransformer) Transform(input *pipeline.RawInput) (*pipeline.TextDocument, error) {
	text, err := extractDOCXText(input)
	if err != nil {
		return nil, fmt.Errorf("extracting docx text: %w", err)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("docx had no extractable text")
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
		Source:  "docx",
		Title:   title,
		Date:    fileModTime(input.Path),
		Content: text,
		Metadata: mergeMeta(input.Metadata, map[string]string{
			"input_file": input.Path,
		}),
	}, nil
}

// docXML matches the minimal structure we care about in document.xml:
// paragraphs containing text runs.
type docXML struct {
	XMLName    xml.Name      `xml:"document"`
	Paragraphs []paragraphXML `xml:"body>p"`
}

type paragraphXML struct {
	Runs []runXML `xml:"r"`
}

type runXML struct {
	Text string `xml:"t"`
}

func extractDOCXText(input *pipeline.RawInput) (string, error) {
	var zr *zip.Reader
	var err error

	if len(input.Content) > 0 {
		zr, err = zip.NewReader(bytes.NewReader(input.Content), int64(len(input.Content)))
	} else if input.Path != "" {
		rc, zerr := zip.OpenReader(input.Path)
		if zerr != nil {
			return "", zerr
		}
		defer rc.Close()
		zr = &rc.Reader
	} else {
		return "", fmt.Errorf("no docx path or content provided")
	}
	if err != nil {
		return "", err
	}

	for _, f := range zr.File {
		if f.Name != "word/document.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return "", err
		}

		var doc docXML
		if err := xml.Unmarshal(data, &doc); err != nil {
			return "", err
		}

		var buf strings.Builder
		for _, p := range doc.Paragraphs {
			for _, r := range p.Runs {
				buf.WriteString(r.Text)
			}
			buf.WriteString("\n")
		}
		return buf.String(), nil
	}

	return "", fmt.Errorf("word/document.xml not found in docx")
}
