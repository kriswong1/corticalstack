// Package transformers contains the individual modality → TextDocument
// implementations used by the pipeline's Stage 1.
package transformers

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/kriswong/corticalstack/internal/pipeline"
)

// readInputBytes returns the input as a UTF-8 string, reading from Path if needed.
func readInputBytes(input *pipeline.RawInput) string {
	if len(input.Content) > 0 {
		return string(input.Content)
	}
	if input.Path != "" {
		data, err := os.ReadFile(input.Path)
		if err != nil {
			return ""
		}
		return string(data)
	}
	return ""
}

// fileModTime returns the mod time of a file, or time.Now() on error.
func fileModTime(path string) time.Time {
	if path == "" {
		return time.Now()
	}
	info, err := os.Stat(path)
	if err != nil {
		return time.Now()
	}
	return info.ModTime()
}

// mergeMeta merges two string maps, preferring extra on collision.
func mergeMeta(base, extra map[string]string) map[string]string {
	merged := make(map[string]string)
	for k, v := range base {
		merged[k] = v
	}
	for k, v := range extra {
		merged[k] = v
	}
	return merged
}

// identifierFor returns a stable identifier for a RawInput.
func identifierFor(input *pipeline.RawInput) string {
	if input.Filename != "" {
		return input.Filename
	}
	if input.Path != "" {
		return filepath.Base(input.Path)
	}
	if input.URL != "" {
		return input.URL
	}
	return fmt.Sprintf("text-%d", time.Now().UnixMilli())
}

// httpGet fetches a URL with a browser-ish UA and a sane timeout.
func httpGet(url string) (string, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) CorticalStack/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("http %d: %s", resp.StatusCode, url)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// --- HTML stripping helpers ---

var (
	scriptRe       = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	styleRe        = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	noscriptRe     = regexp.MustCompile(`(?is)<noscript[^>]*>.*?</noscript>`)
	htmlTagRe      = regexp.MustCompile(`<[^>]*>`)
	htmlTitleRe    = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	htmlEntityRe   = regexp.MustCompile(`&[a-zA-Z0-9#]+;`)
	multiNewlineRe = regexp.MustCompile(`\n{3,}`)
	wsRe           = regexp.MustCompile(`[ \t]+`)
)

// stripHTML returns readable plain text from an HTML document.
func stripHTML(html string) string {
	text := scriptRe.ReplaceAllString(html, "")
	text = styleRe.ReplaceAllString(text, "")
	text = noscriptRe.ReplaceAllString(text, "")
	text = htmlTagRe.ReplaceAllString(text, " ")
	text = decodeCommonEntities(text)
	text = htmlEntityRe.ReplaceAllString(text, "")
	text = wsRe.ReplaceAllString(text, " ")
	text = multiNewlineRe.ReplaceAllString(strings.TrimSpace(text), "\n\n")
	return text
}

// extractHTMLTitle returns the <title> contents, or an empty string.
func extractHTMLTitle(html string) string {
	match := htmlTitleRe.FindStringSubmatch(html)
	if len(match) > 1 {
		return strings.TrimSpace(decodeCommonEntities(match[1]))
	}
	return ""
}

func decodeCommonEntities(s string) string {
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&quot;", "\"")
	s = strings.ReplaceAll(s, "&#39;", "'")
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	return s
}
