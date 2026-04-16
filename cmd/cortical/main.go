// CorticalStack dashboard binary.
//
// Loads .env, wires the pipeline (transformers → Claude CLI → vault
// destinations), and serves the web dashboard on PORT (default 8000).
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kriswong/corticalstack/internal/actions"
	"github.com/kriswong/corticalstack/internal/agent"
	"github.com/kriswong/corticalstack/internal/config"
	"github.com/kriswong/corticalstack/internal/dashboard"
	"github.com/kriswong/corticalstack/internal/documents"
	"github.com/kriswong/corticalstack/internal/integrations"
	"github.com/kriswong/corticalstack/internal/intent"
	"github.com/kriswong/corticalstack/internal/itemusage"
	"github.com/kriswong/corticalstack/internal/meetings"
	"github.com/kriswong/corticalstack/internal/persona"
	"github.com/kriswong/corticalstack/internal/pipeline"
	"github.com/kriswong/corticalstack/internal/pipeline/transformers"
	"github.com/kriswong/corticalstack/internal/prds"
	"github.com/kriswong/corticalstack/internal/projects"
	"github.com/kriswong/corticalstack/internal/prototypes"
	"github.com/kriswong/corticalstack/internal/shapeup"
	"github.com/kriswong/corticalstack/internal/telemetry"
	"github.com/kriswong/corticalstack/internal/usecases"
	"github.com/kriswong/corticalstack/internal/vault"
	"github.com/kriswong/corticalstack/internal/web"
	"github.com/kriswong/corticalstack/internal/web/handlers"
	"github.com/kriswong/corticalstack/internal/web/jobs"
	"github.com/kriswong/corticalstack/internal/web/sse"
)

// shutdownTimeout bounds how long we'll wait for in-flight jobs and
// the HTTP server to drain on SIGINT/SIGTERM before forcing exit.
const shutdownTimeout = 30 * time.Second

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	config.Load()

	rootCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	vaultPath := config.VaultPath()
	if err := os.MkdirAll(vaultPath, 0o700); err != nil {
		slog.Error("creating vault dir", "path", vaultPath, "error", err)
		os.Exit(1)
	}

	// Telemetry: capture every Claude CLI invocation to a JSONL file.
	// Recorder and Reader share the same path so an env-var override
	// can't accidentally diverge between writer and reader.
	usagePath := config.UsageLogPath()
	usageRec, err := telemetry.NewJSONLRecorder(usagePath)
	if err != nil {
		slog.Error("usage recorder", "path", usagePath, "error", err)
		os.Exit(1)
	}
	defer usageRec.Close()
	agent.DefaultRecorder = usageRec
	usageReader := telemetry.NewReader(usagePath)
	slog.Info("usage telemetry", "path", usagePath)

	// Item-tagged telemetry: a sibling JSONL that records only the
	// calls whose Agent.Item was set. Powers the unified dashboard's
	// per-card detail page (selected items → aggregate calls/cost/
	// tokens). Calls without an Item are silently skipped here and
	// still flow through the global usageRec above.
	itemUsagePath := config.ItemUsageLogPath()
	itemUsageRec, err := itemusage.NewJSONLRecorder(itemUsagePath)
	if err != nil {
		slog.Error("item usage recorder", "path", itemUsagePath, "error", err)
		os.Exit(1)
	}
	defer itemUsageRec.Close()
	agent.DefaultItemRecorder = itemUsageRec
	itemUsageReader := itemusage.NewReader(itemUsagePath)
	slog.Info("item usage telemetry", "path", itemUsagePath)

	workingDir, err := os.Getwd()
	if err != nil {
		slog.Error("getting working dir", "error", err)
		os.Exit(1)
	}

	v := vault.New(vaultPath)
	claudeModel := config.ClaudeModel()

	// Integrations registry
	reg := integrations.NewRegistry()
	deepgram := integrations.NewDeepgramClient(config.DeepgramAPIKey())
	if err := reg.Register(deepgram); err != nil {
		slog.Error("register deepgram", "error", err)
		os.Exit(1)
	}

	// Persona loader — bootstrapped from embedded templates on first run.
	personaLoader := persona.New(v)
	initResult, err := personaLoader.InitIfMissing()
	if err != nil {
		slog.Error("persona init", "error", err)
		os.Exit(1)
	}
	if len(initResult.Created) > 0 {
		slog.Info("persona: bootstrapped from templates", "created", initResult.Created)
	}

	// Action store
	actionStore := actions.New(v)
	if err := actionStore.Load(); err != nil {
		slog.Error("loading actions index", "error", err)
		os.Exit(1)
	}
	if err := actionStore.EnsureCentralFile(); err != nil {
		slog.Warn("could not create central ACTION-ITEMS.md", "error", err)
	}

	// v3: ShapeUp — constructed before the pipeline so the ingest flow can
	// route raw product ideas straight into the ShapeUp queue.
	shapeupStore := shapeup.New(v)
	if err := shapeupStore.EnsureFolders(); err != nil {
		slog.Warn("could not create product folders", "error", err)
	}
	shapeupAdvancer := shapeup.NewAdvancer(workingDir, claudeModel, personaLoader)

	// Pipeline wiring
	buildTransformers := func(dg *integrations.DeepgramClient) []pipeline.Transformer {
		return transformers.NewDefault(dg)
	}
	pipe := pipeline.New(v, workingDir, claudeModel, deepgram, buildTransformers, actionStore, shapeupStore, personaLoader)
	pipe.EnsureFolders(v)

	// Projects store
	projectStore := projects.New(v)
	if err := projectStore.Refresh(); err != nil {
		slog.Warn("project discovery failed", "error", err)
	}

	// Intent classifier (Claude CLI)
	classifier := intent.NewClaudeClassifier(workingDir, claudeModel, personaLoader)

	// v3: UseCases
	useCaseStore := usecases.New(v)
	if err := useCaseStore.EnsureFolder(); err != nil {
		slog.Warn("could not create usecases folder", "error", err)
	}
	useCaseGen := usecases.NewGenerator(workingDir, claudeModel, personaLoader)

	// v3: Prototypes
	prototypeStore := prototypes.New(v)
	if err := prototypeStore.EnsureFolder(); err != nil {
		slog.Warn("could not create prototypes folder", "error", err)
	}
	prototypeSynth := prototypes.NewSynthesizer(workingDir, claudeModel, personaLoader)

	// v3: PRDs
	prdStore := prds.New(v)
	if err := prdStore.EnsureFolder(); err != nil {
		slog.Warn("could not create prds folder", "error", err)
	}
	prdRetriever := prds.NewRetriever(v)
	prdSynth := prds.NewSynthesizer(workingDir, claudeModel, prdRetriever, actionStore, personaLoader)

	// v5: Meetings (transcript / audio / note pipeline) — store
	// scanned by the unified dashboard. Notes are dropped into
	// vault/meetings/{transcripts,audio,notes}/ by audio ingest or
	// by hand. The legacy summaries/ folder is still readable.
	meetingsStore := meetings.New(v)
	if err := meetingsStore.EnsureFolder(); err != nil {
		slog.Warn("could not create meetings folders", "error", err)
	}

	// v6: Documents (need / in-progress / final pipeline) — new
	// store added with the unified-dashboard refactor. Wraps the
	// existing vault/documents/ folder with stage-aware listing.
	documentsStore := documents.New(v)
	if err := documentsStore.EnsureFolder(); err != nil {
		slog.Warn("could not create documents folder", "error", err)
	}

	// Jobs + SSE bus (shared by ingest + confirm flows)
	bus := sse.NewEventBus()
	jm := jobs.New(rootCtx, pipe, bus, classifier, projectStore)

	// v4/v6: Dashboard aggregator + 15-minute TTL cache over every store.
	// Meetings and Documents are attached via the With*-style chain
	// because they were added with the unified-dashboard refactor and
	// keeping the original NewAggregator signature stable lets older
	// callers compile unchanged.
	dashAgg := dashboard.NewAggregator(v, actionStore, projectStore, prototypeStore, shapeupStore).
		WithMeetings(meetingsStore).
		WithDocuments(documentsStore)
	dashCache := dashboard.NewCache(dashAgg, dashboard.CacheTTL, nil)

	// Build the handler Deps bundle
	deps := handlers.Deps{
		Vault:           v,
		Pipeline:        pipe,
		Jobs:            jm,
		Bus:             bus,
		Registry:        reg,
		Projects:        projectStore,
		Actions:         actionStore,
		Persona:         personaLoader,
		PersonaInitCreated: initResult.Created,
		ShapeUp:         shapeupStore,
		ShapeUpAdvancer: shapeupAdvancer,
		UseCases:        useCaseStore,
		UseCaseGen:      useCaseGen,
		Prototypes:      prototypeStore,
		PrototypeSynth:  prototypeSynth,
		PRDs:            prdStore,
		PRDSynth:        prdSynth,
		Dashboard:       dashCache,
		Usage:           usageReader,
		Meetings:        meetingsStore,
		Documents:       documentsStore,
		ItemUsage:       itemUsageReader,
	}

	srv, err := web.NewServer(deps)
	if err != nil {
		slog.Error("creating server", "error", err)
		os.Exit(1)
	}

	port := config.Port()
	printBanner(vaultPath, port, deepgram.Configured())

	addr := fmt.Sprintf(":%d", port)
	httpSrv := &http.Server{Addr: addr, Handler: srv.Router}

	go func() {
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("http server", "error", err)
			os.Exit(1)
		}
	}()
	slog.Info("cortical listening", "addr", addr)

	<-rootCtx.Done()
	slog.Info("shutting down")
	stop() // stop receiving further signals — a second Ctrl+C now force-exits

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		slog.Error("http shutdown", "error", err)
	}
	if err := jm.Shutdown(shutdownCtx); err != nil {
		slog.Error("jobs shutdown", "error", err)
	}
	slog.Info("bye")
}

func printBanner(vaultPath string, port int, deepgramOK bool) {
	fmt.Println("┌─ CorticalStack ──────────────────────────────────")
	fmt.Printf("│  vault:    %s\n", vaultPath)
	fmt.Printf("│  port:     %d\n", port)
	fmt.Printf("│  claude:   %s\n", claudeStatus())
	fmt.Printf("│  deepgram: %s\n", deepgramStatus(deepgramOK))
	fmt.Println("│  dashboard: http://localhost:" + fmt.Sprint(port) + "/dashboard")
	fmt.Println("└──────────────────────────────────────────────────")
}

func claudeStatus() string {
	if agent.IsInstalled() {
		return "OK (Paperclip / --print)"
	}
	return "MISSING — extraction will fail (run `claude login`)"
}

func deepgramStatus(configured bool) string {
	if configured {
		return "OK"
	}
	return "not configured — audio ingest disabled"
}
