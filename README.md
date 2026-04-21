# CorticalStack

*Your cortical stack for the things you read, watch, hear, and decide to keep.*

A local Go web app that takes whatever you throw at it — text, files, URLs, audio — and stacks it into your Obsidian vault as a structured note with an intention the system can reason about later.

No cloud. No account. No API bill. Just your machine, the Claude CLI you already have, and the vault you already trust.

## Why

In Altered Carbon, the stack holds everything that makes a person — not just memory, but *why* things mattered. The same idea applies to notes. When you save something, you're doing it for a reason: learning it, tracking research, feeding a project, parking a fact for later. Most tools flatten all of that into "a note." CorticalStack makes the **intention** first-class, so the note that comes out the other end actually matches why you saved it — and future you (or a future Claude session) can find it by reason, not just by keyword.

## What it does

You drop something in. CorticalStack:

1. **Transforms** it to plain text — PDF, DOCX, webpage, YouTube transcript, Zoom / Teams / Meet VTT, audio via Deepgram.
2. **Classifies** the intention with a quick Claude call and shows you a preview so you can confirm or tweak.
3. **Extracts** structured fields based on the intention you chose.
4. **Routes** the result to a dated markdown file in your vault, plus action-item trackers and a daily log.

Five intentions ship out of the box — `learning`, `information`, `research`, `project-application`, and `other` — each with its own section layout. Action items get a UUID and sync across the source note, every project they touch, and a central `ACTION-ITEMS.md` tracker. Flip a checkbox in Obsidian, hit reconcile, and the status catches up.

## How

A local Go binary serves a Chi-based dashboard on `localhost:8000`. All LLM work goes through `claude --print` (the Paperclip pattern), so with a Claude Max subscription, extractions cost **$0**. Audio-only ingest needs a Deepgram API key. Notes land in an Obsidian-compatible vault on your own disk — nothing leaves the machine except the CLI call to Claude and, optionally, the audio blob to Deepgram.

## Scope — where this works

**Designed for:**
- A single user, running locally on their own machine
- Windows (native), macOS, Linux, or Windows + WSL2
- Claude Max subscribers who want Paperclip-style CLI extractions (no `ANTHROPIC_API_KEY` needed)
- Anyone with an Obsidian vault — or just a folder of markdown they like

**Not designed for:**
- Teams, shared servers, or multi-user deployments
- Cloud / hosted environments
- Setups that can only talk to an API key (the Claude CLI is required)

If you need a team knowledge base, this isn't it. If you want something that runs on your laptop and quietly feeds your second brain, read on.

## Prerequisites

- **Go 1.26+** — `go version`
- **Claude CLI** installed and logged in — [claude.ai/download](https://claude.ai/download), then `claude login`
- **Deepgram API key** — only if you plan to ingest audio files or YouTube videos without captions
- An **Obsidian vault** (or any folder you want markdown written into)

## Initial setup

```bash
# 1. Clone
git clone https://github.com/kriswong/corticalstack
cd corticalstack

# 2. Copy the env template
cp .env.example .env

# 3. Edit .env — point VAULT_PATH at your Obsidian vault,
#    add DEEPGRAM_API_KEY if you want audio (see reference below)

# 4. Run it
go run ./cmd/cortical

# 5. Open the dashboard
#    http://localhost:8000/dashboard
```

On first run CorticalStack creates the folders it needs inside your vault (`projects/`, `transcripts/`, `product/raw/`, `.cortical/`, and so on). The fastest sanity check is to paste a URL into the ingest box — you should see the classify preview within a few seconds.

## .env reference

```bash
VAULT_PATH=vault            # or absolute path to an existing Obsidian vault
PORT=8000
CLAUDE_MODEL=               # blank = CLI default
CLAUDE_BIN=                 # optional explicit path to the claude binary
DEEPGRAM_API_KEY=           # only needed for audio / YouTube fallback
```

## Running under WSL2

If Microsoft Defender for Endpoint (or similar EDR) blocks Go binaries on the Windows host, install and run CorticalStack inside a WSL2 Ubuntu distro. Two things to know:

- **Claude CLI location.** The cleanest setup is to install Claude Code inside the distro — `npm i -g @anthropic-ai/claude-code && claude login`. If you'd rather reuse your Windows install, set `CLAUDE_BIN` in `.env` — drive-letter paths like `C:\Users\you\.claude\local\claude.exe` are auto-rewritten to `/mnt/c/...` when WSL2 is detected.
- **Vault location.** Obsidian vaults usually live on the Windows side so they sync through OneDrive / iCloud. CorticalStack auto-translates a Windows `VAULT_PATH` to `/mnt/c/...` under WSL2, so the same `.env` works from either side. Reads and writes across `/mnt/c` are slower than native WSL storage — if your vault is write-heavy, consider keeping it at `~/vault` inside WSL and syncing out with a separate tool.

Localhost forwarding (browsing to `http://localhost:8000/dashboard` from Windows) needs WSL2 1.0 or newer — check with `wsl --version` in PowerShell.

## Contributing

Primarily a personal tool, but pull requests are welcome. For anything bigger than a bug fix, please open an issue first — scope creep is a known failure mode for tools like this, and a short conversation up front usually saves a weekend of rework. Run `go test ./...` and `go vet ./...` before you open a PR.

## License

MIT — see [LICENSE](LICENSE) if/when added.
