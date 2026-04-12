# CorticalStack — Go Best-Practices Audit

**Date:** 2026-04-11
**Scope:** Full sweep of `cmd/` and `internal/` against Go best practices.
**Target:** Go 1.26, Chi + SSE, 3-stage Transform→Extract→Route pipeline, Claude CLI via Paperclip.
**Status:** Phases A+B+C+E+F shipped 2026-04-11 — see Execution Status below.

---

## Execution status — Phases A+B+C+E+F shipped (2026-04-11)

9 commits landed for A+B+C on `origin/master` (`7123178..195966a`). Phases E and F landed same day. Build, vet, and all tests green throughout.

| # | Commit | Covers |
|---|---|---|
| 1 | `5ae0691` fix(shapeup): guard empty artifacts slice | Top-10 #6 |
| 2 | `f0b57af` security: tighten vault file permissions | Top-10 #9 + gitignore fix (see Discovery below) |
| 3 | `ebbae4d` security(transformers): add SSRF guard to httpGet | Top-10 #8 |
| 4 | `c686ce1` refactor(integrations): return error instead of panicking | Top-10 #5 |
| 5 | `1397b4b` refactor: thread ctx through Extract/Classify/ExtractAndRoute | Top-10 #3, #4 (partial) |
| 6 | `ef4577b` refactor: thread ctx through Advance/FromDoc/Synthesize handlers | Top-10 #3, #4 (partial) |
| 7 | `53f851e` refactor(jobs): add root ctx, WaitGroup tracking, and Shutdown | Top-10 #2 |
| 8 | `0e16d5b` feat(main): graceful shutdown on SIGINT/SIGTERM | Top-10 #7 |
| +1 | `195966a` security(vault): tighten actions.go perms missed in earlier sweep | follow-up for #9 |

### Two Top-10 items skipped as false positives

- **#1 — `vault/parser.go:70` nil-deref** — `note.Body` is a `string` (not `[]byte`) and there is already a dominating `if note.Body != ""` guard at line 65. No real risk.
- **Template audit (server.go:59)** — all 12 templates use `html/template` (auto-escaping), no `| html` filters, no raw concatenation. The `template.HTML()` wrap is safe because the inner template already escapes.

### Phase F — Re-audit fixes (14 items)

| # | File | Severity | Problem | Fix |
|---|------|----------|---------|-----|
| F1 | `jobs/manager.go:237` | **P1** | `job.StartedAt` written without mutex — data race | Wrapped in `m.mu.Lock()` |
| F2 | `jobs/manager.go:220-235` | **P1** | `List()`/`Get()` return mutable `*Job` pointers — callers race with goroutines | Added `snapshot()` method, `Get`/`List` return copies |
| F3 | `sse/bus.go:48-57` | **P1** | `Publish` could panic on send-to-closed-channel | Added `recover()` guard in send loop |
| F4 | `handlers/*.go` (multiple) | **P1** | `http.Error()` 500s not logged server-side | Added `slog.Error`/`slog.Warn` before every 500/503 response |
| F5 | `handlers/persona.go:24` | **P1** | `h.Persona.Get()` error silently discarded | Log error, continue with empty content |
| F6 | `handlers/prds.go:14` | **P1** | `h.PRDs.List()` error silently discarded | Same pattern |
| F7 | `jobs/manager.go:110` | **P2** | Unbounded `jobs` map — no submission cap | Added `maxJobs = 10_000` constant + `ErrTooManyJobs` |
| F8 | `handlers/handlers.go:468` | **P2** | Dead code: unused `abs()` helper | Removed |
| F9 | `config/config.go:37-44` | **P2** | Port not validated — could be negative or >65535 | Added `n >= 1 && n <= 65535` range check |
| F10 | `agent/agent.go:115-117` | **P2** | `parseStream` silently skips malformed JSON lines | Log first 3 parse errors via `slog.Warn` |
| F11 | `handlers/handlers.go:299,318,332` | **P2** | Unchecked `w.Write` in SSE `StreamJob` | Check errors, return on failure |
| F12 | `actions/reconcile.go:56-58` | **P2** | File read errors silently skipped during reconciliation | Added `slog.Warn` for non-existence errors |
| F13 | `shapeup/store.go:174-187` | **P2** | Artifact read errors silently skipped during listing | Added `slog.Warn` for skipped artifacts |
| F14 | `extract.go:81`, `classifier.go:82`, `handlers.go:211` | **P2** | Hard-coded limits (50k chars, 8k chars, 200MB upload) | Moved to `config` package with env var overrides |

Also fixed in Phase F: silent error discards in `handlers/shapeup.go`, `handlers/usecases.go`, `handlers/prototypes.go` (same `list, _ = .List()` pattern as F5/F6), and `actions/reconcile.go:129` (`_ = s.Sync(a)` → logged).

### Deferred to follow-up phases

- **Phase D** — test coverage (still ~5%; critical paths in transformers, vault, pipeline, agent remain untested)
- Minor P2 items: SSE subscriber cap, middleware Authorization header sanitization, strict `filepath.Rel()` path-traversal check, `pipeline/templates` sub-package split, exported field doc comments, error-returns in `pipeline/route.go:182`
- Metrics (jobs processed, transform success/fail, extraction duration, cost tracking) — deferred from Phase E scope

### Discovery — `.gitignore` was hiding `internal/vault/`

Not in the original audit. Found during Commit 2 execution: the unanchored `vault/` pattern in `.gitignore` was matching **both** the top-level Obsidian `vault/` directory and the `internal/vault/` Go package. As a result, `internal/vault/{actions.go, daily.go, parser.go, vault.go}` had been completely untracked since the initial commit (2026-04-11).

**Fix** (bundled into commit `f0b57af`): anchored the pattern to root (`vault/` → `/vault/`). The four vault package files are now in git history for the first time.

**Lingering impact:** the first perms sweep only touched the two vault files I'd already edited directly; the follow-up commit `195966a` caught two more `0o644` sites in `vault/actions.go` that became visible only after the package was tracked. Regression sweep now clean.

---

## Top 10 priority fixes

| # | File:Line | Severity | Problem | Status |
|---|-----------|----------|---------|-----|
| 1 | `internal/vault/parser.go:70` | **P1** | Nil-deref on empty `note.Body` | ⏭ **Skipped (false positive)** — already guarded |
| 2 | `internal/web/jobs/manager.go:111,131` | **P1** | `go runPreview/runConfirm` have no cancellation; shutdown abandons Claude processes | ✅ **Shipped** `53f851e` |
| 3 | `internal/pipeline/extract.go:39` | **P1** | `context.Background()` on request path | ✅ **Shipped** `1397b4b` |
| 4 | `internal/intent/classifier.go:38` | **P1** | `context.Background()` in `Classify()` on request path | ✅ **Shipped** `1397b4b` |
| 5 | `internal/integrations/registry.go:45` | **P1** | `panic()` on duplicate registration in library code | ✅ **Shipped** `c686ce1` |
| 6 | `internal/web/handlers/shapeup.go:112` | **P1** | Unchecked slice bounds on `thread.Artifacts[len(...)-1]` | ✅ **Shipped** `5ae0691` |
| 7 | `cmd/cortical/main.go` (missing) | **P1** | No graceful shutdown; SIGINT/SIGTERM unhandled | ✅ **Shipped** `0e16d5b` |
| 8 | `internal/pipeline/transformers/helpers.go:72-92` | **P2** | SSRF: `httpGet` doesn't block private IPs | ✅ **Shipped** `ebbae4d` |
| 9 | `internal/vault/vault.go:52,70` | **P2** | Vault files `0o644` / dirs `0o755` — too loose for private notes | ✅ **Shipped** `f0b57af` + `195966a` |
| 10 | throughout | **P2** | Stdlib `log.Printf` instead of `slog` — no structured logs, no request IDs | ✅ **Shipped** Phase E |

---

## 1. Error handling

**Clean overall** — most pipeline errors wrap with `%w`, sentinel errors defined where needed (e.g., `ErrDeepgramNotConfigured`).

Findings:
- **P1** `internal/integrations/registry.go:45` — `panic()` on duplicate registration. Library code should return error, not panic.
- **P2** `internal/pipeline/route.go:182` — `_ = os.MkdirAll(...)` silently drops errors on vault folder creation.
- **P2** `internal/pipeline/transformers/helpers.go:76` — `defer resp.Body.Close()` without error check (acceptable, but worth noting if reads are critical).

## 2. context.Context

Five request-path functions hardcode `context.Background()` — cancellation from client disconnect or server shutdown never reaches Claude CLI invocations, which can run up to 10+ minutes (`internal/agent/agent.go:34`).

- **P1** `internal/pipeline/extract.go:39` — should take `ctx` param.
- **P1** `internal/intent/classifier.go:38` — same.
- **P1** `internal/shapeup/advance.go`, `internal/usecases/generate.go`, `internal/prototypes/synthesize.go` — all use `context.Background()` for Claude CLI calls.
- **P1** `internal/web/jobs/manager.go:111,131` — goroutines spawned without context; long-running Claude calls have no graceful shutdown path.

**Clean**: `StreamJob` handler correctly uses `r.Context()` (`web/handlers/handlers.go:303`).

## 3. Concurrency

- **P1** `internal/web/jobs/manager.go:111,131` — `go m.runPreview()` / `go m.runConfirm()` have no cancellation. On shutdown, Claude processes are orphaned. Fix: pass `context.Context`, use `sync.WaitGroup` for lifecycle.
- **P2** `internal/web/sse/bus.go:37-43` — channel close only happens on explicit `Unsubscribe()`. `StreamJob` defers correctly (`:296`), but any other caller that forgets leaks.
- **P2** `internal/web/sse/bus.go:17` — `EventBus.subscribers` map has no cap; malicious client could exhaust memory by repeatedly subscribing.

**Clean**: `sync.RWMutex` used correctly in vault/actions/persona/jobs.

## 4. Project layout

**Clean** — no findings worth fixing.

- `cmd/cortical/` contains only `main.go`.
- `internal/` split is cohesive (15 packages, no "utils" dumping ground).
- No circular imports.

**P2** `internal/pipeline/` is the largest package (15 types + extract/transform/route + 4 template files). Cohesion is high, but splitting templates into `pipeline/templates` would tidy things up. Low priority.

## 5. Safety / defensive coding

- **P1** `internal/vault/parser.go:70` — `note.Body[len(note.Body)-1]` on empty body. Guard with `len(note.Body) > 0`.
- **P1** `internal/web/handlers/shapeup.go:112` — `thread.Artifacts[len(thread.Artifacts)-1].ID` assumes non-empty slice.

**Clean**: type assertions use comma-ok (`handlers.go:313`), `defer file.Close()` consistently applied, integer conversions are safe for realistic input sizes.

## 6. Security

- **Safe** `internal/agent/agent.go:68-69` — `claude --print` args are hardcoded and prompt is piped via stdin. **No shell injection.**
- **Adequate** `internal/web/handlers/handlers.go:358-362` — path traversal check against `..`. Could be stricter using `filepath.Rel()` to verify path stays within vault root.
- **P2** `internal/pipeline/transformers/helpers.go:72-92` — `httpGet()` fetches arbitrary URLs with 10 MB cap, but no SSRF guard. Add check using `net.ParseIP` to reject private ranges before request.
- **P2** `internal/web/server.go:59` — `template.HTML()` wraps content for render. Verify all `.html` templates use auto-escaping (`{{.Field}}`) and never interpolate user input via `{{.Field | html}}` or concatenation.
- **P2** `internal/vault/vault.go:52,70` — files written with `0o644`, directories `0o755`. Vault contains private notes; tighten to `0o600` / `0o700`.
- **P2** Middleware logs don't sanitize `Authorization` headers. Filter before logging.

## 7. Testing

**Coverage ~5%.** Three test files (`actions/store_test.go`, `actions/markdown_test.go`, `projects/store_test.go`) for ~70 source files. Existing tests use good table-driven patterns.

**Missing critical-path tests:**
- `pipeline/transform.go`, `pipeline/extract.go` — zero coverage on core pipeline
- `pipeline/transformers/*.go` — no tests for PDF, DOCX, webpage, YouTube, Deepgram transformers
- `web/handlers/*.go` — no integration tests for ingest, job lifecycle, SSE
- `vault/*.go` — no tests for read/write, parser, daily log
- `agent/agent.go` — no tests for Claude CLI invocation (needs mocking)

**Recommendation:** Phase dedicated to transformers + vault + pipeline tests. Use `testify` (already idiomatic per `golang-stretchr-testify` skill) and table-driven patterns.

## 8. Observability

- **P2** Throughout — stdlib `log.Printf` / `log.Fatalf` used instead of `slog` (Go 1.21+). Examples: `cmd/cortical/main.go`, `internal/web/middleware/middleware.go:32`, `internal/persona/loader.go:122`.
- **P2** No request IDs threaded through logs — impossible to correlate a job's lifecycle across middleware, handler, pipeline, agent.
- **P2** Log levels flat — startup errors and warnings both use `log.Fatalf()` / `log.Printf()` indifferently.
- **P1** No metrics. A Claude-dependent system should track at minimum: jobs processed, transform success/fail by type, extraction duration, and token/cost estimates.

## 9. Code style / naming / idioms

**Clean** — strong idiom compliance:

- Exported identifiers have doc comments where it matters.
- Receiver names consistent (`v *Vault`, `p *Pipeline`, `s *Store`).
- No interface-stuttering, no receiver-stuttering.
- "Accept interfaces, return structs" followed.
- Modern Go: `any`, `errors.Is`, no leftover `interface{}`.

**P2** Some exported struct fields in `pipeline/types.go` (`Extracted`, `ProcessResult`, `TextDocument`) lack field-level doc comments.

## 10. Design patterns

- **Clean** DI via `handlers.Deps` bundle in `main.go:123-141` — excellent wiring, minimal globals.
- **Clean** Interfaces minimal and focused (`Transformer`, `Extractor`, `Destination`, `Integration`).
- **P1** **Missing graceful shutdown.** `cmd/cortical/main.go` calls `http.ListenAndServe` and blocks. No signal handling, no `srv.Shutdown(ctx)`. Combined with #2 (uncontrolled job goroutines), SIGINT leaves zombie `claude` processes.
- **P2** Minimal config validation. `godotenv.Load()` runs but there's no `config.Validate()` catching missing `VAULT_PATH` / `CLAUDE_MODEL` until first use.

---

## Recommended phase clusters

**Phase A — Lifecycle & context hygiene** ✅ shipped
Covered #2, #3, #4, #5, #7. Commits: `c686ce1`, `1397b4b`, `ef4577b`, `53f851e`, `0e16d5b`.

**Phase B — Safety patches** ✅ shipped
Covered #6 (#1 skipped as false positive). Commit: `5ae0691`.

**Phase C — Security hardening** ✅ shipped
Covered #8, #9 (template audit skipped — already safe). Commits: `ebbae4d`, `f0b57af`, `195966a`.

**Phase D — Test coverage** ⏳ deferred
Dedicated phase. Table-driven tests for transformers, vault, pipeline stages, agent (with Claude CLI mock). Current coverage ~5% (3 test files).

**Phase E — Observability** ✅ shipped
Migrated all `log.*` and `fmt.Fprintf(os.Stderr)` to `slog`. Added `RequestID` middleware (UUID per request, `X-Request-ID` header, context propagation). Added structured logging to job lifecycle (status transitions, completion duration, failures) and Claude CLI invocations (model, prompt length, duration, cost). Proper log levels throughout (Error for fatal startup, Warn for non-fatal, Info for operational). Metrics deferred to a future phase.

**Phase F — Re-audit hardening** ✅ shipped
14-item sweep from second full-codebase audit. Fixed data races in job manager (mutex on `StartedAt`, snapshot copies from `Get`/`List`), SSE bus send-on-closed-channel guard, server-side error logging for all 500/503 HTTP responses, silent error discards in 6 handler page-render paths, unbounded job map cap (10k), dead code removal, port validation, `parseStream` error logging, SSE `w.Write` error checking, reconcile/shapeup silent-skip logging, and 3 hard-coded limits moved to config with env var overrides.

---

## Summary

**Strengths:** clean project layout, strong DI, safe shell-out pattern, proper error wrapping, minimal globals, idiomatic naming, structured logging with request IDs, defensive concurrency.

**Weaknesses remaining:** test coverage (~5%), metrics. All Top-10 items and Phase F re-audit items are resolved.

**Estimated Top-10 effort:** ~8 hours (Phases A+B+C bundled) — **actual: ~8 hours, 9 commits shipped 2026-04-11.** Phases E and F shipped same day. Phase D (test coverage) remains as the primary outstanding effort.
