# CorticalStack

A local Go web app that ingests raw inputs, classifies the user's intention,
extracts structured metadata via the Claude CLI, and writes the result as an
Obsidian-compatible markdown note. Pipeline: **Transform → Classify → Extract → Route**.

## Why

Every note you save has a reason behind it — you're learning something,
capturing reference material, doing research for a project, or feeding a
project directly. v2 makes that intention first-class so the output structure
matches *why* you saved it, and so future Claude sessions can pull project-
relevant context out of the vault by frontmatter and tag.

## Features

- **Inputs**: pasted text, file uploads (`txt`, `md`, `pdf`, `docx`, `html`,
  `vtt`), URLs (generic webpages, YouTube, LinkedIn), and audio (`mp3`, `wav`,
  `m4a`, `ogg`, `flac`, `webm`) via Deepgram.
- **Two-phase ingest**: every submission first runs Transform + a fast Claude
  classification call, then pauses on a preview modal so you can confirm or
  edit the proposed intention, suggested projects, and title before the full
  extraction runs.
- **Intentions** (5 templates):
  - `learning` — Summary · Key Points · How This Applies · Open Questions
  - `information` — Facts · Claims · Definitions · Key Points
  - `research` — Findings · Sources · Relevance · Next Steps
  - `project-application` — Impact · Action Items · Integration Notes · Next Steps
  - `other` — Claude proposes the section structure
- **Projects**: discovered from `vault/projects/<id>/project.md`. Create new
  projects from the dashboard; the manifest and an `ACTION-ITEMS.md` file are
  written into the vault.
- **Smart action items**: every action gets a stable UUID and is written to
  three locations — the source note, every associated project's tracker, and
  the central `ACTION-ITEMS.md`. Six statuses (`pending`, `ack`, `doing`,
  `done`, `deferred`, `cancelled`). Status changes from the dashboard
  propagate to all locations; status changes you make in Obsidian directly
  can be re-synced via `POST /api/actions/reconcile`.
- **YouTube via `kkdai/youtube/v2`**: pure-Go captions extraction with
  timestamp formatting; falls back to downloading the lowest-bitrate audio
  stream and pushing it through Deepgram when no captions are available.
- **Meeting transcripts (VTT)**: drop a `.vtt` export from Zoom, Teams, Google
  Meet, or Otter and the dedicated transformer strips the WEBVTT header, NOTE
  blocks, cue identifiers, timestamps, and styling markup while preserving
  speaker prefixes. Participants land in `authors` and the last cue's end time
  lands in `duration`. Notes file under `vault/transcripts/`.
- **ShapedPRD queue**: every extraction — regardless of transformer or
  intention — asks Claude for any product, feature, or workflow idea raised in
  the content. Each idea becomes a new raw artifact in the ShapeUp pipeline
  (`vault/product/raw/`), starting its own thread with a backlink to the
  source note, ready to be advanced through frame → shape → breadboard →
  pitch.
- **Extraction**: uses the Claude CLI in Paperclip mode (`claude --print`),
  so it runs at $0/call on a Claude Max subscription. No `ANTHROPIC_API_KEY`
  required.
- **UI**: local Chi dashboard with live SSE progress streaming, vault library
  browser, projects page, actions tracker. Dark Altered Carbon design system,
  no framework.

## Prerequisites

- Go 1.25+ (`go version`)
- [Claude CLI](https://claude.ai/download) with `claude login` completed
- A Deepgram API key (only needed for audio ingest or YouTube fallback)

## Quick start

```bash
git clone https://github.com/kriswong/corticalstack
cd corticalstack
cp .env.example .env    # fill in VAULT_PATH, DEEPGRAM_API_KEY (optional)
go run ./cmd/cortical
```

Then open <http://localhost:8000/dashboard>.

## .env

```bash
VAULT_PATH=vault            # or absolute path to an existing Obsidian vault
PORT=8000
CLAUDE_MODEL=               # blank = CLI default
CLAUDE_BIN=                 # optional explicit path to the claude binary
DEEPGRAM_API_KEY=           # required for audio ingest and YouTube fallback
```

## Running under WSL2

On Windows machines where Microsoft Defender for Endpoint (or similar EDR)
blocks the binary from running on the host OS, install and run CorticalStack
inside a WSL2 Ubuntu distro instead. Two things need attention:

**1. Where is the Claude CLI?** CorticalStack resolves `claude` in this
order — the first match wins:

1. `$CLAUDE_BIN` if set (drive-letter paths like
   `C:\Users\kris\.claude\local\claude.exe` are auto-rewritten to
   `/mnt/c/...` when running in WSL2)
2. `claude` on `$PATH`
3. Native home-directory candidates (`~/.claude/local/claude.exe`, etc.)
4. WSL2 fallback: glob `/mnt/c/Users/*/...` for the standard install layouts

The cleanest setup is to install Claude Code inside the WSL2 distro:

```bash
# inside your WSL2 Ubuntu shell
npm i -g @anthropic-ai/claude-code
claude login
```

If you prefer to reuse the Windows-side install, set `CLAUDE_BIN` in
`.env` to its path, either as a Windows path or the mounted form:

```bash
CLAUDE_BIN=C:\Users\kris\.claude\local\claude.exe
# or
CLAUDE_BIN=/mnt/c/Users/kris/.claude/local/claude.exe
```

**2. Where is the vault?** Obsidian vaults usually live on the Windows
filesystem so they sync via OneDrive/iCloud/etc. From WSL2 that path is
accessible under `/mnt/c/...`. CorticalStack auto-translates a Windows
drive-letter `VAULT_PATH` when it detects WSL2, so you can share the
same `.env` between native Windows and WSL2:

```bash
# Both forms work when running under WSL2:
VAULT_PATH=C:\Users\kris\Documents\Obsidian\MyVault
VAULT_PATH=/mnt/c/Users/kris/Documents/Obsidian/MyVault
```

**Performance note.** Reads and writes across the `/mnt/c` boundary are
slower than native WSL filesystem access, and directory-level file
watching on the Windows side does not propagate to WSL. CorticalStack
does not use fsnotify, so this only affects ingest throughput — not
correctness. If the vault is write-heavy, consider keeping it inside
the WSL filesystem (`~/vault`) and syncing out with a separate tool.

**WSL2 version.** Localhost forwarding (so you can open
`http://localhost:8000/dashboard` from a Windows browser) requires
WSL2 1.0 or newer. Check with `wsl --version` in PowerShell.

## Ingest flow

```
POST /api/ingest/{text,url,file}
      │
      ▼
[1] Transform               (transformer picks first CanHandle match)
      │
      ▼
TextDocument
      │
      ▼
[1.5] Classify              (claude --print: intention + projects + title)
      │
      ▼
job_status = awaiting_confirmation
      │
User confirms or edits intention/projects/title in the dashboard
      │
POST /api/jobs/{id}/confirm
      │
      ▼
[2] Extract                 (intention-aware claude --print prompt)
      │
      ▼
Extracted (intention-specific fields)
      │
      ▼
[3] Route ──▶ $VAULT_PATH/<folder>/YYYY-MM-DD_slug.md  (intention template)
      │  ──▶ vault/.cortical/actions.json              (canonical index)
      │  ──▶ vault/ACTION-ITEMS.md                     (central tracker)
      │  ──▶ vault/projects/<id>/ACTION-ITEMS.md       (per-project)
      │  ──▶ vault/product/raw/YYYY-MM-DD_slug.md      (one per extracted idea)
      └─ ──▶ vault/daily/YYYY-MM-DD.md                 (daily log line)
```

## Project layout

```
cmd/cortical/                   # entry point
internal/
  agent/                        # Claude CLI subprocess wrapper (Paperclip)
  config/                       # .env loading
  integrations/                 # third-party clients (Deepgram, extensible)
  intent/                       # Claude classifier (single-shot preview)
  projects/                     # vault-backed project store
  actions/                      # smart action store + multi-location sync
  pipeline/                     # 3-stage pipeline
    transformers/               # one file per input modality
    template_*.go               # 5 intention renderers + shared helpers
    extract.go                  # intention-aware extraction prompts
    route.go                    # destinations (vault note, actions, daily)
  vault/                        # Obsidian vault I/O
  web/                          # Chi HTTP server
    handlers/                   # dashboard, ingest, jobs, projects, actions
    jobs/                       # two-phase async job manager
    sse/                        # pub/sub event bus
    middleware/                 # recovery + request logger
    templates/                  # HTML templates (embedded)
    static/                     # CSS + JS (embedded)
```

## Action item format

The canonical action line in any markdown file:

```markdown
- [ ] [Owner] Description *(due: 2026-04-18)* #status/pending <!-- id:abc123-... -->
```

- The checkbox tracks Obsidian-native done/undone.
- The `#status/*` tag carries the nuanced state (Obsidian renders the tag
  natively and you can search by it).
- The HTML comment holds the stable UUID so the multi-location sync can find
  the same action across files.

Status reconciliation: ticking the checkbox in Obsidian updates the line but
not the JSON index until you click **Reconcile from Obsidian** on the actions
page (or call `POST /api/actions/reconcile`). Status changes made in the
dashboard write through to every location automatically.

## Transformers (in priority order)

| Name          | Inputs                                  | Notes                                                  |
|---------------|-----------------------------------------|--------------------------------------------------------|
| `deepgram`    | Audio files                             | Requires `DEEPGRAM_API_KEY`                            |
| `vtt`         | `.vtt` meeting transcripts              | Zoom / Teams / Meet / Otter; preserves speakers        |
| `pdf`         | `.pdf`                                  | Pure Go (`ledongthuc/pdf`)                             |
| `docx`        | `.docx`                                 | Pure Go (unzip + parse `document.xml`)                 |
| `youtube`     | YouTube URLs                            | `kkdai/youtube/v2` captions; Deepgram audio fallback   |
| `linkedin`    | LinkedIn post/article URLs              | JSON-LD + HTML stripping                               |
| `webpage`     | Generic `http(s)://` URLs               | Stdlib HTTP + HTML stripping                           |
| `html`        | `.html` files / pasted HTML             | Regex-based HTML stripping                             |
| `passthrough` | Plain text, `.txt`, `.md`               | Catch-all                                              |

## Tests

```bash
go test ./...
go vet ./...
```

Unit tests cover:
- `internal/actions` — markdown round-trip, store CRUD, multi-location sync, status counts
- `internal/projects` — create, list, refresh, duplicate detection

## License

MIT — see [LICENSE](LICENSE) if/when added.
