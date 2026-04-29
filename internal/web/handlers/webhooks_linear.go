package handlers

import (
	"io"
	"log/slog"
	"net/http"

	"github.com/kriswong/corticalstack/internal/config"
	"github.com/kriswong/corticalstack/internal/integrations/linear"
)

// LinearWebhook handles POST /webhooks/linear.
//
// Pipeline:
//   1. Read raw body (HMAC verifies against the wire bytes, not the
//      decoded JSON — order/whitespace differences would break otherwise).
//   2. Verify the Linear-Signature header against LINEAR_WEBHOOK_SECRET.
//      Tampered or missing signature → 401, no mutation.
//   3. Hand off to the dispatcher, which decodes + routes by Type.
//
// The dispatcher's idempotency cache suppresses replays so Linear's
// retry-on-non-2xx behavior doesn't apply the same change twice.
func (h *Handler) LinearWebhook(w http.ResponseWriter, r *http.Request) {
	if h.LinearWebhooks == nil {
		http.Error(w, "linear webhooks not initialized", http.StatusServiceUnavailable)
		return
	}
	secret := config.LinearWebhookSecret()
	if secret == "" {
		http.Error(w, "LINEAR_WEBHOOK_SECRET not configured", http.StatusServiceUnavailable)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20)) // 1MB cap
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}

	sig := r.Header.Get(linear.WebhookHeaderSignature)
	if !linear.VerifySignature(body, sig, secret) {
		// Don't echo why — keep attackers blind to whether it was a
		// signature mismatch vs missing header etc.
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	if err := h.LinearWebhooks.Dispatcher.Dispatch(body); err != nil {
		slog.Warn("linear webhook dispatch failed", "error", err)
		// Linear retries on 5xx — return 200 for content errors so they
		// don't loop forever, but log so we can see what slipped through.
	}
	w.WriteHeader(http.StatusOK)
}
