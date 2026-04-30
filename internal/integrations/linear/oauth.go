package linear

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Linear OAuth 2.0 endpoints. Authorize is browser-facing (linear.app);
// token exchange and revocation hit the API host.
const (
	OAuthAuthorizeURL = "https://linear.app/oauth/authorize"
	OAuthTokenURL     = "https://api.linear.app/oauth/token"
	OAuthRevokeURL    = "https://api.linear.app/oauth/revoke"
)

// DefaultScopes covers what CorticalStack needs end-to-end:
//   - read: ListTeams, ListInitiatives, ListProjects, FetchViewer
//   - write: project / issue / document / initiative / milestone mutations
const DefaultScopes = "read,write"

// GenerateState returns a 32-byte random string for CSRF protection
// on the OAuth roundtrip. URL-safe base64 with padding stripped.
func GenerateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// BuildAuthorizeURL composes the URL the browser is redirected to so
// the user can approve the OAuth app. Scope is comma-separated per
// Linear's docs. `prompt=consent` forces the consent screen on every
// run, which is the safer default for a tool that may be reauthorized
// against different workspaces.
func BuildAuthorizeURL(clientID, redirectURI, state, scopes string) string {
	if scopes == "" {
		scopes = DefaultScopes
	}
	q := url.Values{}
	q.Set("client_id", clientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("response_type", "code")
	q.Set("scope", scopes)
	q.Set("state", state)
	q.Set("prompt", "consent")
	return OAuthAuthorizeURL + "?" + q.Encode()
}

// TokenResponse is Linear's OAuth token-exchange wire shape. Linear
// access tokens are long-lived (tens of years) and have no
// refresh_token; we only persist access_token.
type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
	Scope       string `json:"scope"`
}

// ExchangeCodeForToken trades the authorization code returned to the
// callback for an access token. Linear expects form-encoded body and
// returns JSON.
func ExchangeCodeForToken(ctx context.Context, clientID, clientSecret, code, redirectURI string) (*TokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)

	req, err := http.NewRequestWithContext(ctx, "POST", OAuthTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("linear oauth: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("linear oauth: token request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("linear oauth: read response: %w", err)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("linear oauth: http %d: %s", resp.StatusCode, truncate(string(body), 500))
	}

	var t TokenResponse
	if err := json.Unmarshal(body, &t); err != nil {
		return nil, fmt.Errorf("linear oauth: decode token: %w", err)
	}
	if strings.TrimSpace(t.AccessToken) == "" {
		return nil, fmt.Errorf("linear oauth: empty access_token in response")
	}
	return &t, nil
}

// RevokeToken asks Linear to invalidate an access token. Best-effort:
// the local Disconnect path still wipes credentials even if the remote
// revoke fails (offline, expired, etc.).
func RevokeToken(ctx context.Context, accessToken string) error {
	if strings.TrimSpace(accessToken) == "" {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, "POST", OAuthRevokeURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	httpClient := &http.Client{Timeout: 5 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("linear oauth revoke: http %d: %s", resp.StatusCode, truncate(string(body), 200))
}

// BuildRedirectURI joins the configured base URL with the callback
// path. Strips trailing slash on the base so we don't end up with
// "//oauth/...".
func BuildRedirectURI(baseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	return base + "/oauth/linear/callback"
}
