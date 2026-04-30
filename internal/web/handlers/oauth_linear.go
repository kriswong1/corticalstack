package handlers

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/kriswong/corticalstack/internal/config"
	"github.com/kriswong/corticalstack/internal/integrations/linear"
)

const (
	oauthStateCookie = "linear_oauth_state"
	oauthStateTTL    = 10 * time.Minute
)

// StartLinearOAuth kicks off the OAuth roundtrip. Generates a random
// state, sets it as an HttpOnly cookie, and 302-redirects the browser
// to Linear's authorize URL.
//
// GET /oauth/linear/start
func (h *Handler) StartLinearOAuth(w http.ResponseWriter, r *http.Request) {
	clientID := strings.TrimSpace(config.LinearOAuthClientID())
	clientSecret := strings.TrimSpace(config.LinearOAuthClientSecret())
	if clientID == "" || clientSecret == "" {
		http.Error(w, "linear oauth not configured: set LINEAR_OAUTH_CLIENT_ID and LINEAR_OAUTH_CLIENT_SECRET first", http.StatusBadRequest)
		return
	}

	state, err := linear.GenerateState()
	if err != nil {
		internalError(w, "linear.oauth.state", err)
		return
	}

	secure := r.TLS != nil || strings.HasPrefix(strings.ToLower(config.CorticalBaseURL()), "https://")
	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookie,
		Value:    state,
		Path:     "/oauth/linear",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(oauthStateTTL),
		MaxAge:   int(oauthStateTTL.Seconds()),
	})

	redirectURI := linear.BuildRedirectURI(config.CorticalBaseURL())
	authorizeURL := linear.BuildAuthorizeURL(clientID, redirectURI, state, linear.DefaultScopes)
	http.Redirect(w, r, authorizeURL, http.StatusFound)
}

// LinearOAuthCallback handles Linear's redirect after the user
// approves (or rejects) the authorization request. Exchanges the code
// for an access token, persists it to .env, and bounces the browser
// back to /config so the LinearCard re-renders in the connected state.
//
// GET /oauth/linear/callback?code=...&state=...
func (h *Handler) LinearOAuthCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if errCode := q.Get("error"); errCode != "" {
		desc := q.Get("error_description")
		h.redirectToConfig(w, r, "linear_error", errCode+": "+desc)
		return
	}
	code := strings.TrimSpace(q.Get("code"))
	state := strings.TrimSpace(q.Get("state"))
	if code == "" || state == "" {
		h.redirectToConfig(w, r, "linear_error", "missing code or state")
		return
	}

	cookie, err := r.Cookie(oauthStateCookie)
	if err != nil || cookie.Value == "" {
		h.redirectToConfig(w, r, "linear_error", "missing state cookie (start the flow again)")
		return
	}
	if cookie.Value != state {
		h.redirectToConfig(w, r, "linear_error", "state mismatch (possible CSRF; start the flow again)")
		return
	}
	// Clear the state cookie regardless of outcome.
	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookie,
		Value:    "",
		Path:     "/oauth/linear",
		HttpOnly: true,
		Secure:   cookie.Secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})

	clientID := strings.TrimSpace(config.LinearOAuthClientID())
	clientSecret := strings.TrimSpace(config.LinearOAuthClientSecret())
	redirectURI := linear.BuildRedirectURI(config.CorticalBaseURL())

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	tok, err := linear.ExchangeCodeForToken(ctx, clientID, clientSecret, code, redirectURI)
	if err != nil {
		h.redirectToConfig(w, r, "linear_error", err.Error())
		return
	}

	if err := config.SetEnvAndPersist("LINEAR_OAUTH_TOKEN", tok.AccessToken); err != nil {
		internalError(w, "linear.oauth.persist_token", err)
		return
	}
	// Personal API key and OAuth token are mutually exclusive paths;
	// once OAuth has produced a token, drop any leftover personal key
	// so authHeader doesn't pick the wrong scheme.
	if config.LinearAPIKey() != "" {
		_ = config.SetEnvAndPersist("LINEAR_API_KEY", "")
	}
	if li := h.linearIntegration(); li != nil {
		li.SetOAuthToken(tok.AccessToken)
	}

	h.redirectToConfig(w, r, "linear_connected", "")
}

// redirectToConfig sends the browser back to the config page with an
// optional query flag the SPA can read to show a toast.
func (h *Handler) redirectToConfig(w http.ResponseWriter, r *http.Request, key, value string) {
	url := "/config"
	if key != "" {
		url += "?" + key
		if value != "" {
			url += "=" + escapeForQuery(value)
		}
	}
	http.Redirect(w, r, url, http.StatusFound)
}

// escapeForQuery is a tiny URL-component escape that keeps query
// values readable. Use net/url for full safety; this is a redirect
// flag, not a security boundary.
func escapeForQuery(s string) string {
	r := strings.NewReplacer(" ", "+", "&", "%26", "?", "%3F", "#", "%23")
	return r.Replace(s)
}
