// Package transformers contains the individual modality → TextDocument
// implementations used by the pipeline's Stage 1.
package transformers

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/kriswong/corticalstack/internal/pipeline"
)

// safeHTTPClient is a package-level client that blocks connections to
// private / loopback / link-local IP ranges at dial time, catching
// SSRF attempts including DNS rebinding (the check runs after the
// resolver so any rebind is caught before the TCP handshake).
var safeHTTPClient = &http.Client{
	Timeout: 30 * time.Second,
	Transport: &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
			ControlContext: func(ctx context.Context, network, address string, c syscall.RawConn) error {
				host, _, err := net.SplitHostPort(address)
				if err != nil {
					return fmt.Errorf("parsing address: %w", err)
				}
				ip := net.ParseIP(host)
				if ip == nil {
					return fmt.Errorf("resolved address is not an IP: %q", host)
				}
				if isPrivateIP(ip) {
					return fmt.Errorf("blocked private IP %s", ip)
				}
				return nil
			},
		}).DialContext,
	},
}

// isPrivateIP reports whether ip is a loopback, private, link-local,
// or otherwise non-routable address that must not be reachable from
// server-side URL fetchers.
func isPrivateIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	// Go 1.17+ covers RFC 1918 and the IPv6 ULA range.
	if ip.IsPrivate() {
		return true
	}
	// CGNAT range (RFC 6598) — not covered by IsPrivate.
	if ip4 := ip.To4(); ip4 != nil && ip4[0] == 100 && ip4[1]&0xc0 == 64 {
		return true
	}
	return false
}

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

// httpGet fetches a URL with a browser-ish UA, a sane timeout, and
// an SSRF guard that rejects private / loopback / link-local IPs.
func httpGet(url string) (string, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) CorticalStack/1.0")
	resp, err := safeHTTPClient.Do(req)
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
