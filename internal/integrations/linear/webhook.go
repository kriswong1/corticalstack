package linear

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/kriswong/corticalstack/internal/actions"
)

// WebhookHeaderSignature is the HTTP header Linear uses for the HMAC
// signature on every outbound webhook delivery.
const WebhookHeaderSignature = "Linear-Signature"

// WebhookPayload is the envelope Linear delivers for every event. The
// `data` field is type-dependent; we keep it raw and decode on demand
// inside the dispatcher.
//
// See https://developers.linear.app/docs/graphql/webhooks for the
// canonical schema. We capture only the fields we route on.
type WebhookPayload struct {
	Action         string          `json:"action"`         // "create" / "update" / "remove"
	Type           string          `json:"type"`           // "Issue" / "Project" / "ProjectUpdate" / "Document" / etc.
	Data           json.RawMessage `json:"data"`
	URL            string          `json:"url"`
	WebhookID      string          `json:"webhookId,omitempty"`
	OrganizationID string          `json:"organizationId,omitempty"`
	CreatedAt      string          `json:"createdAt,omitempty"`
}

// linearStateRef is the inline state reference Linear includes on
// Issue payloads. Defined as a named type so functions can take/return
// it without re-spelling the struct literal.
type linearStateRef struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
	Type string `json:"type,omitempty"`
}

// IssueData is the subset of Linear's Issue payload we route on. All
// fields optional because Linear may include only the changed subset
// on update events.
type IssueData struct {
	ID         string          `json:"id"`
	Title      string          `json:"title,omitempty"`
	Identifier string          `json:"identifier,omitempty"`
	Priority   int             `json:"priority,omitempty"`
	Estimate   int             `json:"estimate,omitempty"`
	DueDate    string          `json:"dueDate,omitempty"`
	State      *linearStateRef `json:"state,omitempty"`
	Assignee   *struct {
		ID    string `json:"id,omitempty"`
		Name  string `json:"name,omitempty"`
		Email string `json:"email,omitempty"`
	} `json:"assignee,omitempty"`
}

// VerifySignature implements Linear's HMAC-SHA256 verification scheme:
// the header carries hex-encoded HMAC of the raw request body using
// the registered webhook secret as key. Returns true on match,
// constant-time-comparing per crypto/hmac.Equal.
//
// Returns false on any error (malformed hex, key/length mismatch) so
// callers can treat it as a single boolean gate.
func VerifySignature(rawBody []byte, header, secret string) bool {
	header = strings.TrimSpace(header)
	if header == "" || secret == "" {
		return false
	}
	expected, err := hex.DecodeString(header)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(rawBody)
	return hmac.Equal(expected, mac.Sum(nil))
}

// dedupeCache is a small fixed-size set of webhook IDs we've already
// processed. Linear retries on transient receiver errors, so without
// this we'd apply the same state change multiple times.
type dedupeCache struct {
	mu   sync.Mutex
	seen map[string]time.Time
	max  int
}

func newDedupeCache(max int) *dedupeCache {
	return &dedupeCache{seen: make(map[string]time.Time, max), max: max}
}

func (c *dedupeCache) check(id string) bool {
	if id == "" {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.seen[id]; ok {
		return true
	}
	if len(c.seen) >= c.max {
		// Evict entries older than 1h. If still full, drop arbitrary
		// keys until under capacity. O(1) amortized insert.
		cutoff := time.Now().Add(-time.Hour)
		for k, t := range c.seen {
			if t.Before(cutoff) {
				delete(c.seen, k)
			}
		}
		for k := range c.seen {
			if len(c.seen) < c.max {
				break
			}
			delete(c.seen, k)
		}
	}
	c.seen[id] = time.Now()
	return false
}

// LastReceivedAt is exposed so the integration status endpoint can
// report when the most recent webhook landed.
type LastReceivedAt struct {
	mu sync.RWMutex
	at time.Time
}

func (l *LastReceivedAt) Set(t time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.at = t
}

func (l *LastReceivedAt) Get() time.Time {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.at
}

// WebhookDispatcher routes a verified webhook to the right local
// store. Pure logic — the HTTP wrapper is in the handler package so
// this stays testable without an http.ResponseWriter.
type WebhookDispatcher struct {
	Stores         SyncStores
	dedupe         *dedupeCache
	LastReceivedAt *LastReceivedAt
}

// NewWebhookDispatcher wires a dispatcher.
func NewWebhookDispatcher(stores SyncStores) *WebhookDispatcher {
	return &WebhookDispatcher{
		Stores:         stores,
		dedupe:         newDedupeCache(1000),
		LastReceivedAt: &LastReceivedAt{},
	}
}

// Dispatch parses + routes a payload. The webhookId guard suppresses
// replays. Returns nil for replays + unsupported types (no-op).
func (d *WebhookDispatcher) Dispatch(rawBody []byte) error {
	var p WebhookPayload
	if err := json.NewDecoder(bytes.NewReader(rawBody)).Decode(&p); err != nil {
		return fmt.Errorf("decode payload: %w", err)
	}
	if d.dedupe.check(p.WebhookID) {
		return nil
	}
	d.LastReceivedAt.Set(time.Now())

	switch p.Type {
	case "Issue":
		return d.dispatchIssue(p)
	case "Project", "ProjectUpdate", "Document":
		// Workflow telemetry / vault-owned content — no local state to
		// mutate per docs/linear/README.md §3 Fork D conflict policy.
		return nil
	}
	return nil
}

// dispatchIssue applies workflow-owned field changes to the matching
// local Action. Per Q5 lock: status, priority, deadline, owner are
// Linear-owned; title/description stay vault-owned.
func (d *WebhookDispatcher) dispatchIssue(p WebhookPayload) error {
	if d.Stores.Actions == nil {
		return nil
	}
	var data IssueData
	if err := json.Unmarshal(p.Data, &data); err != nil {
		return fmt.Errorf("decode issue data: %w", err)
	}
	if data.ID == "" {
		return nil
	}
	targetID := d.findActionIDByLinearID(data.ID)
	if targetID == "" {
		// Issue may exist in the workspace but not be one CorticalStack
		// created. Not an error.
		return nil
	}

	patch := actions.ActionPatch{}
	if status := mapLinearStateTypeToStatus(data.State); status != "" {
		patch.Status = actions.Status(status)
	}
	if data.Priority > 0 {
		patch.Priority = mapLinearPriorityToActionPriority(data.Priority)
	}
	if data.Assignee != nil {
		patch.Owner = firstNonEmptyStr(data.Assignee.Name, data.Assignee.Email)
	}
	if data.DueDate != "" {
		dl := data.DueDate
		patch.Deadline = &dl
	}

	if _, err := d.Stores.Actions.Update(targetID, patch); err != nil {
		return fmt.Errorf("update action: %w", err)
	}
	return nil
}

// findActionIDByLinearID is a linear scan over the action store. Cheap
// enough for the action volumes we expect (low thousands).
func (d *WebhookDispatcher) findActionIDByLinearID(linearID string) string {
	for _, a := range d.Stores.Actions.List() {
		if a.LinearIssueID == linearID {
			return a.ID
		}
	}
	return ""
}

// mapLinearStateTypeToStatus maps Linear's state.type (the stable
// machine-readable category) to a CorticalStack action.Status string.
// Returns empty string when the state shouldn't override.
func mapLinearStateTypeToStatus(state *linearStateRef) string {
	if state == nil {
		return ""
	}
	switch state.Type {
	case "backlog":
		return "inbox"
	case "unstarted":
		return "next"
	case "started":
		return "doing"
	case "completed":
		return "done"
	case "canceled":
		return "cancelled"
	}
	return ""
}

// mapLinearPriorityToActionPriority is the inverse of priorityToLinear
// in mapper.go. Linear's 1=urgent / 2=high collapse to p1.
func mapLinearPriorityToActionPriority(p int) actions.Priority {
	switch p {
	case 1, 2:
		return actions.PriorityHigh
	case 3:
		return actions.PriorityMedium
	case 4:
		return actions.PriorityLow
	}
	return ""
}
