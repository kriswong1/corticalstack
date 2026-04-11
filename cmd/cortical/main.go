// CorticalStack dashboard binary.
//
// Loads .env, wires the pipeline (transformers → Claude CLI → vault
// destinations), and serves the web dashboard on PORT (default 8000).
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/kriswong/corticalstack/internal/actions"
	"github.com/kriswong/corticalstack/internal/agent"
	"github.com/kriswong/corticalstack/internal/config"
	"github.com/kriswong/corticalstack/internal/integrations"
	"github.com/kriswong/corticalstack/internal/intent"
	"github.com/kriswong/corticalstack/internal/pipeline"
	"github.com/kriswong/corticalstack/internal/pipeline/transformers"
	"github.com/kriswong/corticalstack/internal/prds"
	"github.com/kriswong/corticalstack/internal/projects"
	"github.com/kriswong/corticalstack/internal/prototypes"
	"github.com/kriswong/corticalstack/internal/shapeup"
	"github.com/kriswong/corticalstack/internal/usecases"
	"github.com/kriswong/corticalstack/internal/vault"
	"github.com/kriswong/corticalstack/internal/web"
	"github.com/kriswong/corticalstack/internal/web/handlers"
	"github.com/kriswong/corticalstack/internal/web/jobs"
	"github.com/kriswong/corticalstack/internal/web/sse"
)

func main() {
	config.Load()

	vaultPath := config.VaultPath()
	if err := os.MkdirAll(vaultPath, 0o755); err != nil {
		log.Fatalf("creating vault dir %q: %v", vaultPath, err)
	}

	workingDir, err := os.Getwd()
	if err != nil {
		log.Fatalf("getting working dir: %v", err)
	}

	v := vault.New(vaultPath)
	claudeModel := config.ClaudeModel()

	// Integrations registry
	reg := integrations.NewRegistry()
	deepgram := integrations.NewDeepgramClient(config.DeepgramAPIKey())
	reg.Register(deepgram)

	// Action store
	actionStore := actions.New(v)
	if err := actionStore.Load(); err != nil {
		log.Fatalf("loading actions index: %v", err)
	}
	if err := actionStore.EnsureCentralFile(); err != nil {
		log.Printf("warning: could not create central ACTION-ITEMS.md: %v", err)
	}

	// Pipeline wiring
	buildTransformers := func(dg *integrations.DeepgramClient) []pipeline.Transformer {
		return transformers.NewDefault(dg)
	}
	pipe := pipeline.New(v, workingDir, claudeModel, deepgram, buildTransformers, actionStore)
	pipe.EnsureFolders(v)

	// Projects store
	projectStore := projects.New(v)
	if err := projectStore.Refresh(); err != nil {
		log.Printf("warning: project discovery failed: %v", err)
	}

	// Intent classifier (Claude CLI)
	classifier := intent.NewClaudeClassifier(workingDir, claudeModel)

	// v3: ShapeUp
	shapeupStore := shapeup.New(v)
	if err := shapeupStore.EnsureFolders(); err != nil {
		log.Printf("warning: could not create product folders: %v", err)
	}
	shapeupAdvancer := shapeup.NewAdvancer(workingDir, claudeModel)

	// v3: UseCases
	useCaseStore := usecases.New(v)
	if err := useCaseStore.EnsureFolder(); err != nil {
		log.Printf("warning: could not create usecases folder: %v", err)
	}
	useCaseGen := usecases.NewGenerator(workingDir, claudeModel)

	// v3: Prototypes
	prototypeStore := prototypes.New(v)
	if err := prototypeStore.EnsureFolder(); err != nil {
		log.Printf("warning: could not create prototypes folder: %v", err)
	}
	prototypeSynth := prototypes.NewSynthesizer(workingDir, claudeModel)

	// v3: PRDs
	prdStore := prds.New(v)
	if err := prdStore.EnsureFolder(); err != nil {
		log.Printf("warning: could not create prds folder: %v", err)
	}
	prdRetriever := prds.NewRetriever(v)
	prdSynth := prds.NewSynthesizer(workingDir, claudeModel, prdRetriever, actionStore)

	// Jobs + SSE bus (shared by ingest + confirm flows)
	bus := sse.NewEventBus()
	jm := jobs.New(pipe, bus, classifier, projectStore)

	// Build the handler Deps bundle
	deps := handlers.Deps{
		Vault:           v,
		Pipeline:        pipe,
		Jobs:            jm,
		Bus:             bus,
		Registry:        reg,
		Projects:        projectStore,
		Actions:         actionStore,
		ShapeUp:         shapeupStore,
		ShapeUpAdvancer: shapeupAdvancer,
		UseCases:        useCaseStore,
		UseCaseGen:      useCaseGen,
		Prototypes:      prototypeStore,
		PrototypeSynth:  prototypeSynth,
		PRDs:            prdStore,
		PRDSynth:        prdSynth,
	}

	srv, err := web.NewServer(deps)
	if err != nil {
		log.Fatalf("creating server: %v", err)
	}

	port := config.Port()
	printBanner(vaultPath, port, deepgram.Configured())

	addr := fmt.Sprintf(":%d", port)
	if err := http.ListenAndServe(addr, srv.Router); err != nil {
		log.Fatalf("http server: %v", err)
	}
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
