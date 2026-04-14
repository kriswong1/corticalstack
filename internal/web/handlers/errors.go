package handlers

import (
	"log/slog"
	"net/http"
)

// internalError logs the underlying error at Error level with an operation
// tag and writes a generic "internal error" response with status 500. Use
// this for any 500-level response where the wrapped error may contain
// filesystem paths, vault internals, or other details a client shouldn't
// see. The operation tag should be a short, stable string the dev can
// grep the server log for — e.g. "prd.list", "usecase.from_doc",
// "persona.get".
//
// For user-actionable validation errors (400 BadRequest, 404 NotFound),
// keep using `http.Error(w, err.Error(), status)` — those short messages
// are safe and give the user something to act on.
func internalError(w http.ResponseWriter, op string, err error) {
	slog.Error(op, "error", err)
	http.Error(w, "internal error", http.StatusInternalServerError)
}

// serviceUnavailable is the same as internalError but returns 503. Used
// for temporary failures like a saturated job queue or an unreachable
// downstream service. Same logging + client-safe message discipline.
func serviceUnavailable(w http.ResponseWriter, op string, err error) {
	slog.Error(op, "error", err)
	http.Error(w, "service unavailable", http.StatusServiceUnavailable)
}
