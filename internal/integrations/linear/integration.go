package linear

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Integration adapts Client to the integrations.Integration interface
// (defined in internal/integrations/registry.go). Wraps Client so the
// config card's SetEnvAndPersist flow can mutate APIKey on the live
// instance without re-registering.
type Integration struct {
	mu     sync.RWMutex
	Client *Client
	// TeamKey is the LINEAR_TEAM_KEY env value, the default team for
	// every sync. Held here so the live process picks up Save without
	// a restart, mirroring DeepgramClient.APIKey.
	TeamKey string
}

// NewIntegration wraps an existing client.
func NewIntegration(client *Client, teamKey string) *Integration {
	return &Integration{Client: client, TeamKey: teamKey}
}

// Webhooks holds the live dispatcher + last-received timestamp. Wired
// in main.go so the inbound /webhooks/linear handler and the status
// endpoint share state.
type Webhooks struct {
	Dispatcher *WebhookDispatcher
}

// NewWebhooks constructs a webhook subsystem bound to the given stores.
func NewWebhooks(stores SyncStores) *Webhooks {
	return &Webhooks{Dispatcher: NewWebhookDispatcher(stores)}
}

// LastReceivedAt is a convenience accessor.
func (w *Webhooks) LastReceivedAt() time.Time {
	if w == nil || w.Dispatcher == nil {
		return time.Time{}
	}
	return w.Dispatcher.LastReceivedAt.Get()
}

// ID implements integrations.Integration.
func (i *Integration) ID() string { return "linear" }

// Name implements integrations.Integration.
func (i *Integration) Name() string { return "Linear" }

// Configured reports whether either credential (OAuth token or
// personal API key) is present. Team key is optional at this layer —
// sync orchestration will fail loudly later if it's missing when needed.
func (i *Integration) Configured() bool {
	i.mu.RLock()
	defer i.mu.RUnlock()
	if i.Client == nil {
		return false
	}
	return strings.TrimSpace(i.Client.APIKey) != "" ||
		strings.TrimSpace(i.Client.OAuthToken) != ""
}

// AuthMode reports how the live client is currently authenticated:
// "oauth", "key", or "" when not configured. OAuth wins when both are
// set, mirroring Client.authHeader.
func (i *Integration) AuthMode() string {
	i.mu.RLock()
	defer i.mu.RUnlock()
	if i.Client == nil {
		return ""
	}
	if strings.TrimSpace(i.Client.OAuthToken) != "" {
		return "oauth"
	}
	if strings.TrimSpace(i.Client.APIKey) != "" {
		return "key"
	}
	return ""
}

// HealthCheck calls Viewer with a short timeout. Used by the integration
// status endpoint and the config card's Test button.
func (i *Integration) HealthCheck() error {
	if !i.Configured() {
		return fmt.Errorf("linear: not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := i.Client.FetchViewer(ctx); err != nil {
		return fmt.Errorf("linear health check: %w", err)
	}
	return nil
}

// SetCredentials atomically updates the API key and team key.
// Called by the SaveLinear handler after SetEnvAndPersist so the
// running process picks up the new values without a restart.
//
// Setting a non-empty API key clears any OAuth token — the user has
// explicitly chosen the personal-API-key path and the previous OAuth
// session should not silently keep authenticating.
func (i *Integration) SetCredentials(apiKey, teamKey string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.Client == nil {
		i.Client = NewClient(apiKey)
	} else {
		i.Client.APIKey = apiKey
		if strings.TrimSpace(apiKey) != "" {
			i.Client.OAuthToken = ""
		}
	}
	i.TeamKey = teamKey
}

// SetOAuthToken atomically swaps the live client onto an OAuth access
// token. Clears the personal API key so authHeader picks the right
// scheme.
func (i *Integration) SetOAuthToken(token string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.Client == nil {
		i.Client = NewOAuthClient(token)
		return
	}
	i.Client.OAuthToken = token
	i.Client.APIKey = ""
}

// ClearCredentials wipes both the OAuth token and the API key on the
// live client. Used by the Disconnect handler.
func (i *Integration) ClearCredentials() {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.Client == nil {
		return
	}
	i.Client.OAuthToken = ""
	i.Client.APIKey = ""
}

// CurrentTeamKey is a thread-safe read of the configured team key.
func (i *Integration) CurrentTeamKey() string {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.TeamKey
}
