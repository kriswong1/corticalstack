// Package transformers contains the individual modality → TextDocument
// implementations used by the pipeline's Stage 1.
package transformers

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
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

// readInputBytes returns the input as a UTF-8 string, reading from Path if
// needed. LO-02: propagates file read errors so callers can distinguish
// "empty file" from "permission denied / EOF / etc." — the old behavior
// of returning "" on any error made those two cases indistinguishable
// downstream (every transformer reported "no content" regardless of the
// real reason).
func readInputBytes(input *pipeline.RawInput) (string, error) {
	if len(input.Content) > 0 {
		return string(input.Content), nil
	}
	if input.Path != "" {
		data, err := os.ReadFile(input.Path)
		if err != nil {
			return "", fmt.Errorf("read %s: %w", input.Path, err)
		}
		return string(data), nil
	}
	return "", nil
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

// validateSafeURL resolves pageURL's host and returns an error if any
// resolved IP is loopback, private, link-local, or otherwise non-routable.
// Used as a pre-flight SSRF guard for code paths that cannot install a
// dial-time hook (e.g. headless Chrome). Only http/https are allowed.
func validateSafeURL(pageURL string) error {
	u, err := url.Parse(pageURL)
	if err != nil {
		return fmt.Errorf("parse url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("blocked scheme %q (only http/https allowed)", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("url has no host: %s", pageURL)
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", host, err)
	}
	if len(ips) == 0 {
		return fmt.Errorf("no IPs for host %s", host)
	}
	for _, ip := range ips {
		if isPrivateIP(ip) {
			return fmt.Errorf("blocked private IP %s for host %s", ip, host)
		}
	}
	return nil
}

// isRequestURLSafe parses a CDP-intercepted request URL and returns nil if the
// request's resolved host is routable (not loopback / private / link-local /
// etc.). This is the core decision used by the chromedp request interceptor,
// so it is kept pure and unit-testable in isolation from the browser.
//
// Only http/https/ws/wss are allowed. Data URLs, file URLs, chrome-extension,
// etc. are rejected. DNS lookup errors are treated as "blocked" — a strict
// fail-closed policy so a resolver-trickery attack cannot slip past.
func isRequestURLSafe(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parse url: %w", err)
	}
	scheme := strings.ToLower(u.Scheme)
	switch scheme {
	case "http", "https", "ws", "wss":
		// allowed, continue to host check
	case "data", "blob", "about":
		// These are browser-internal; safe and must not be blocked or
		// chromedp's page init (about:blank) breaks.
		return nil
	default:
		return fmt.Errorf("blocked scheme %q", scheme)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("url has no host: %s", rawURL)
	}
	// If the host is already a literal IP, check it directly — no DNS.
	if literal := net.ParseIP(host); literal != nil {
		if isPrivateIP(literal) {
			return fmt.Errorf("blocked private IP literal %s", literal)
		}
		return nil
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", host, err)
	}
	if len(ips) == 0 {
		return fmt.Errorf("no IPs for host %s", host)
	}
	for _, ip := range ips {
		if isPrivateIP(ip) {
			return fmt.Errorf("blocked private IP %s for host %s", ip, host)
		}
	}
	return nil
}

// chromedpAllowNoSandbox is read from the environment at chromedpGet time.
// If set to "1", the --no-sandbox flag is passed to Chrome and a loud warning
// is logged. This is required inside unprivileged Linux containers that can't
// use user namespaces, but is strongly discouraged on normal desktops/servers
// because it removes the process-level mitigation against browser exploits.
const chromedpAllowNoSandboxEnv = "CHROMEDP_ALLOW_NO_SANDBOX"

// chromedpGet renders a URL in headless Chrome and returns the full HTML
// after JavaScript execution. Used as a fallback when httpGet returns
// empty or JS-dependent content (SPAs, JS redirects, etc.).
//
// SSRF defense is layered:
//
//  1. Pre-flight: validateSafeURL rejects the initial target if it resolves
//     to a private / loopback / link-local IP.
//  2. In-browser request interception: every HTTP(S)/WS request the page
//     issues is paused via Fetch.enable and inspected in a ListenTarget
//     handler. If the URL (after DNS resolve) points at a private IP, the
//     request is failed with Fetch.failRequest(BlockedByClient). This covers
//     in-page fetch/XHR, JS redirects, sub-resources (<img>, <script>,
//     <iframe>), and HTTP 3xx redirects — every follow-up goes through the
//     same listener.
//  3. Sandbox: --no-sandbox is NOT passed by default. It is only added when
//     CHROMEDP_ALLOW_NO_SANDBOX=1 is set in the environment (container use),
//     and a warning is logged each time.
//  4. Surface reduction: extensions, plugins, background networking,
//     translation, and default apps are disabled.
func chromedpGet(pageURL string) (string, error) {
	if err := validateSafeURL(pageURL); err != nil {
		return "", fmt.Errorf("chromedp ssrf guard: %w", err)
	}
	return chromedpRender(pageURL)
}

// chromedpRender is the browser-launch + navigation core of chromedpGet,
// split out so tests can exercise the request-interception layer (defense
// #2) directly against a loopback test server without being short-circuited
// by the pre-flight validateSafeURL check (defense #1). Production callers
// MUST go through chromedpGet which applies both layers.
func chromedpRender(pageURL string) (string, error) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		// Surface reduction: disable features not needed to render a page
		// we just want HTML out of. These also shrink the attack surface.
		chromedp.Flag("disable-extensions", true),
		chromedp.Flag("disable-plugins", true),
		chromedp.Flag("disable-background-networking", true),
		chromedp.Flag("disable-sync", true),
		chromedp.Flag("disable-translate", true),
		chromedp.Flag("disable-default-apps", true),
		chromedp.Flag("disable-features", "Translate,NetworkPrediction"),
		// Belt-and-braces: block DNS resolution of "localhost" so even if the
		// interceptor listener somehow misses a request (shouldn't happen
		// because Fetch.enable is in Run before Navigate), the resolver still
		// refuses. Literal 127.0.0.1 is not affected; the interceptor catches
		// literal private IPs.
		chromedp.Flag("host-resolver-rules", "MAP localhost ~NOTFOUND, MAP *.localhost ~NOTFOUND"),
	)
	if os.Getenv(chromedpAllowNoSandboxEnv) == "1" {
		log.Printf("WARNING: chromedp launched with --no-sandbox because %s=1 is set; "+
			"this removes the process-level mitigation for browser exploits and should "+
			"only be used inside an unprivileged container", chromedpAllowNoSandboxEnv)
		opts = append(opts, chromedp.Flag("no-sandbox", true))
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer allocCancel()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Install the request interceptor BEFORE any navigation so the very
	// first request (the target page) is also covered by the live check.
	// Listener must not block on CDP calls (chromedp docs: "Actions must
	// run via a separate goroutine"), so FailRequest/ContinueRequest are
	// dispatched from a goroutine.
	var blockedCount int64
	chromedp.ListenTarget(ctx, func(ev any) {
		paused, ok := ev.(*fetch.EventRequestPaused)
		if !ok {
			return
		}
		requestID := paused.RequestID
		reqURL := ""
		if paused.Request != nil {
			reqURL = paused.Request.URL
		}
		go func() {
			// For the integration test path we expose a hook that whitelists
			// the test's loopback page. In production the hook is nil and
			// every request goes through the normal isRequestURLSafe check.
			safe := isRequestURLSafe(reqURL)
			if safe != nil && chromedpAllowTestURL != nil && chromedpAllowTestURL(reqURL) {
				safe = nil
			}
			if safe != nil {
				atomic.AddInt64(&blockedCount, 1)
				log.Printf("chromedp ssrf block: %s (%v)", reqURL, safe)
				_ = fetch.FailRequest(requestID, network.ErrorReasonBlockedByClient).Do(ctx)
				return
			}
			_ = fetch.ContinueRequest(requestID).Do(ctx)
		}()
	})

	var html string
	err := chromedp.Run(ctx,
		// Enable request interception for every request this target makes.
		// No patterns == intercept everything.
		fetch.Enable(),
		chromedp.Navigate(pageURL),
		chromedp.WaitReady("body"),
		chromedp.Sleep(2*time.Second),
		chromedp.OuterHTML("html", &html),
	)
	if err != nil {
		return "", fmt.Errorf("chromedp render: %w", err)
	}
	if n := atomic.LoadInt64(&blockedCount); n > 0 {
		log.Printf("chromedp: blocked %d in-page request(s) to private/loopback hosts on %s", n, pageURL)
	}
	return html, nil
}

// chromedpAllowTestURL is a package-private hook for integration tests that
// need to let a specific loopback URL through the request interceptor so the
// rest of the interceptor logic (blocking other loopback targets) can be
// exercised end-to-end. It MUST remain nil in production; tests set and
// unset it inside t.Cleanup. A misuse here is obvious because it's a
// package-private, un-exported variable — linters and grep will find it.
var chromedpAllowTestURL func(rawURL string) bool

// contentLooksEmpty returns true if stripped text is too short to be
// meaningful content — indicates the page likely needs JS rendering.
func contentLooksEmpty(text string) bool {
	return len(strings.TrimSpace(text)) < 100
}

// --- HTML stripping helpers ---

var (
	scriptRe       = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	styleRe        = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	noscriptRe     = regexp.MustCompile(`(?is)<noscript[^>]*>.*?</noscript>`)
	navRe          = regexp.MustCompile(`(?is)<nav[^>]*>.*?</nav>`)
	headerRe       = regexp.MustCompile(`(?is)<header[^>]*>.*?</header>`)
	footerRe       = regexp.MustCompile(`(?is)<footer[^>]*>.*?</footer>`)
	svgRe          = regexp.MustCompile(`(?is)<svg[^>]*>.*?</svg>`)
	htmlTagRe      = regexp.MustCompile(`<[^>]*>`)
	htmlTitleRe    = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	htmlEntityRe   = regexp.MustCompile(`&[a-zA-Z0-9#]+;`)
	multiNewlineRe = regexp.MustCompile(`\n{2,}`)
	multiSpaceRe   = regexp.MustCompile(`[ \t]+`)
	blankLineRe    = regexp.MustCompile(`(?m)^\s+$`)
)

// stripHTML returns readable plain text from an HTML document.
func stripHTML(html string) string {
	text := scriptRe.ReplaceAllString(html, "")
	text = styleRe.ReplaceAllString(text, "")
	text = noscriptRe.ReplaceAllString(text, "")
	text = navRe.ReplaceAllString(text, "")
	text = headerRe.ReplaceAllString(text, "")
	text = footerRe.ReplaceAllString(text, "")
	text = svgRe.ReplaceAllString(text, "")
	text = htmlTagRe.ReplaceAllString(text, "\n")
	text = decodeCommonEntities(text)
	text = htmlEntityRe.ReplaceAllString(text, "")
	text = multiSpaceRe.ReplaceAllString(text, " ")
	text = blankLineRe.ReplaceAllString(text, "")
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
