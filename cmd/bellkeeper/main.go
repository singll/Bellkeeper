package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/singll/bellkeeper/internal/app"
	"github.com/singll/bellkeeper/internal/config"
	"github.com/singll/bellkeeper/internal/llmclient"
	"github.com/singll/bellkeeper/internal/llmgateway"
	"github.com/singll/bellkeeper/internal/middleware"
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/pkb"
	"github.com/singll/bellkeeper/internal/repository"
	"github.com/spf13/cobra"
)

var (
	cfgFile string
	version = "1.0.0"

	// pkb-curate flags
	pkbDryRun bool
	pkbRescan bool
	pkbPerRun int
	pkbCfgDir string

	// pkb-curate digest flags
	pkbDigestDomain   string
	pkbDigestPeriod   string
	pkbDigestSince    string
	pkbDigestMaxCards int

	// pkb-curate audit flags
	pkbAuditJSON bool

	// pkb-curate eval flags
	pkbEvalJSON      bool
	pkbEvalTolerance int

	// pkb-curate skeleton flags
	pkbSkeletonTOCFile string

	// pkb-curate fill flags
	pkbFillPerRun int

	// pkb-curate feed flags
	pkbFeedDate       string
	pkbFeedNoPromote  bool
	pkbFeedWriteDaily bool
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "bellkeeper",
		Short: "Bellkeeper - Knowledge Management System",
		Long:  `Bellkeeper is a knowledge management system for collecting, organizing, and retrieving information.`,
	}

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./config/bellkeeper.yaml)")

	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the Bellkeeper server",
		Run:   runServer,
	}

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("Bellkeeper version %s\n", version)
		},
	}

	migrateCmd := &cobra.Command{
		Use:   "migrate",
		Short: "Run database migrations",
		Run:   runMigrate,
	}

	pkbCurateCmd := &cobra.Command{
		Use:   "pkb-curate",
		Short: "Run one PKB maintenance pass (score → triage → reconstruct → reindex)",
		Long: `pkb-curate scores raw articles via the LLM Proxy, triages them into archive/vault
by the thresholds in config/pkb/domains.yaml, reconstructs high-value ones into
Obsidian cards, then rebuilds the search index. It runs once and exits.
Steer behavior by editing config/pkb/ (domains, prompts, thresholds) — no rebuild needed.`,
		Run: runPkbCurate,
	}
	pkbCurateCmd.Flags().BoolVar(&pkbDryRun, "dry-run", false, "score and print decisions without moving/writing files or reindexing")
	pkbCurateCmd.Flags().BoolVar(&pkbRescan, "rescan", false, "rescan ALL raw articles incl. already-processed (default: skip processed for idempotency)")
	pkbCurateCmd.Flags().IntVar(&pkbPerRun, "per-run", 0, "max articles to process this run (0 = use domains.yaml defaults.per_run)")
	pkbCurateCmd.Flags().StringVar(&pkbCfgDir, "pkb-config", "config/pkb", "directory holding domains.yaml + prompts/")

	pkbDigestCmd := &cobra.Command{
		Use:   "digest",
		Short: "Synthesize domain-level PKB digest notes from high-score vault cards",
		Long: `digest reads high-score vault cards by domain, asks the LLM to synthesize
a low-frequency Obsidian digest note, writes it under vault/<domain>/digest/,
then rebuilds the search index. Use --dry-run first to inspect candidates.`,
		Run: runPkbDigest,
	}
	pkbDigestCmd.Flags().BoolVar(&pkbDryRun, "dry-run", false, "print candidate cards without calling the digest LLM or writing files")
	pkbDigestCmd.Flags().StringVar(&pkbDigestDomain, "domain", "", "domain name/display to digest (default: all)")
	pkbDigestCmd.Flags().StringVar(&pkbDigestPeriod, "period", "weekly", "digest period: weekly or monthly")
	pkbDigestCmd.Flags().StringVar(&pkbDigestSince, "since", "", "lower bound date YYYY-MM-DD (default: start of current period)")
	pkbDigestCmd.Flags().IntVar(&pkbDigestMaxCards, "max-cards", 50, "max high-score cards per domain")
	pkbDigestCmd.Flags().StringVar(&pkbCfgDir, "pkb-config", "config/pkb", "directory holding domains.yaml + prompts/")
	pkbCurateCmd.AddCommand(pkbDigestCmd)

	pkbAuditCmd := &cobra.Command{
		Use:   "audit",
		Short: "Audit vault knowledge network health (orphan cards, broken links, hubs, duplicates)",
		Long: `audit scans the vault, builds a link graph, and reports network health metrics.
It is read-only: no files are moved, written, or deleted.
Use --json for machine-readable output.`,
		Run: runPkbAudit,
	}
	pkbAuditCmd.Flags().BoolVar(&pkbAuditJSON, "json", false, "output JSON for machine parsing")
	pkbAuditCmd.Flags().StringVar(&pkbCfgDir, "pkb-config", "config/pkb", "directory holding domains.yaml + prompts/")
	pkbCurateCmd.AddCommand(pkbAuditCmd)

	pkbSkeletonCmd := &cobra.Command{
		Use:   "skeleton <domain>",
		Short: "Generate the initial knowledge skeleton (target concept tree) for a domain from its scope",
		Long: `skeleton reads a domain's scope (one-line direction) from config/pkb/domains.yaml
and asks the skeleton_model to design a multi-level target concept tree where every
node starts as a [缺口] (gap). It writes the tree to vault/<domain>/_index.md via
guardrail-6 (snapshot old index → atomic replace). Existing card links are re-attached
later by the match step; topic MOCs emerge from digest as cards accumulate.

The domain must have a scope: line in domains.yaml first. Use --dry-run to preview.`,
		Args: cobra.ExactArgs(1),
		Run:  runPkbSkeleton,
	}
	pkbSkeletonCmd.Flags().BoolVar(&pkbDryRun, "dry-run", false, "print scope and target path without calling the LLM or writing files")
	pkbSkeletonCmd.Flags().StringVar(&pkbSkeletonTOCFile, "toc-file", "", "optional authoritative-source TOC file to guide tree structure and coverage")
	pkbSkeletonCmd.Flags().StringVar(&pkbCfgDir, "pkb-config", "config/pkb", "directory holding domains.yaml + prompts/")
	pkbCurateCmd.AddCommand(pkbSkeletonCmd)

	pkbMatchCmd := &cobra.Command{
		Use:   "match <domain>",
		Short: "Place a domain's atomic cards onto its knowledge skeleton; unmatched cards go to the waitlist",
		Long: `match reads a domain's skeleton (_index.md), collects all atomic cards, and asks the
match_model which skeleton node each card belongs to. Matched cards fill their node
([缺口] → [[card]]); unmatched cards go to _待归位.md. The skeleton structure is preserved
exactly — only node mount markers are recomputed. Writes via guardrail-6 (snapshot → atomic).

The domain must already have a skeleton (run skeleton first). Use --dry-run to preview.`,
		Args: cobra.ExactArgs(1),
		Run:  runPkbMatch,
	}
	pkbMatchCmd.Flags().BoolVar(&pkbDryRun, "dry-run", false, "print placement decisions without calling the LLM or writing files")
	pkbMatchCmd.Flags().StringVar(&pkbCfgDir, "pkb-config", "config/pkb", "directory holding domains.yaml + prompts/")
	pkbCurateCmd.AddCommand(pkbMatchCmd)

	pkbProposeCmd := &cobra.Command{
		Use:   "propose <domain>",
		Short: "Propose skeleton structure changes from a domain's waitlist; gate by impact radius",
		Long: `propose reads a domain's waitlist (_待归位.md) and current skeleton, asks the
skeleton_model to propose structure changes (add/delete/merge/restructure), then gates
by impact radius (cards touched): small changes (<= skeleton_change_approval_threshold)
are snapshotted and applied automatically; large changes are saved as pending proposals
and pushed to Matrix for !pkb approve. Use --dry-run to preview.`,
		Args: cobra.ExactArgs(1),
		Run:  runPkbPropose,
	}
	pkbProposeCmd.Flags().BoolVar(&pkbDryRun, "dry-run", false, "print the proposal and impact radius without applying, saving, or pushing")
	pkbProposeCmd.Flags().StringVar(&pkbCfgDir, "pkb-config", "config/pkb", "directory holding domains.yaml + prompts/")
	pkbCurateCmd.AddCommand(pkbProposeCmd)

	pkbFillCmd := &cobra.Command{
		Use:   "fill <domain>",
		Short: "Fill skeleton [缺口] gap nodes into V2-verified atomic cards (top-down)",
		Long: `fill walks a domain's skeleton (_index.md), takes its [缺口] (gap) nodes top-down
(breadth-first), and for each: gapfill_model drafts an atomic card and proposes authoritative
sources; the source is fetched (honoring crawl cooling via next_allowed_at) and verify_model
checks whether the page supports the card. Cards land with source/verification/confidence
frontmatter (verified/unverified/llm-only) and are placed back onto the skeleton (缺口 → 已填).

Gated per domain by gap_fill_enabled in domains.yaml. Use --dry-run to list the target gaps.`,
		Args: cobra.ExactArgs(1),
		Run:  runPkbFill,
	}
	pkbFillCmd.Flags().BoolVar(&pkbDryRun, "dry-run", false, "list the gaps this run would fill without drafting/fetching/writing")
	pkbFillCmd.Flags().IntVar(&pkbFillPerRun, "per-run", 0, "max gaps to fill this run (0 = use domains.yaml gap_fill_per_run)")
	pkbFillCmd.Flags().StringVar(&pkbCfgDir, "pkb-config", "config/pkb", "directory holding domains.yaml + prompts/")
	pkbCurateCmd.AddCommand(pkbFillCmd)

	pkbFeedCmd := &cobra.Command{
		Use:   "feed",
		Short: "Build the daily feed-base archive from today's news-type articles (per domain)",
		Long: `feed collects the day's ingested news-type articles (pkb_type in feed_content_types)
from the raw/archive layers, groups them by domain, asks promote_model to summarize "what
happened today" per domain, and writes <feed-root>/<domain>/<date>.md (one file per day,
append-only across days). Feed items are not stored as standalone atomic cards (ADR-0005).

Requires a feed-container domain (feed: true) in domains.yaml. Use --dry-run to list items.`,
		Args: cobra.NoArgs,
		Run:  runPkbFeed,
	}
	pkbFeedCmd.Flags().BoolVar(&pkbDryRun, "dry-run", false, "list the feed items this run would summarize without calling the LLM or writing files")
	pkbFeedCmd.Flags().StringVar(&pkbFeedDate, "date", "", "target date YYYY-MM-DD (default: today)")
	pkbFeedCmd.Flags().BoolVar(&pkbFeedNoPromote, "no-promote", false, "only build the feed archive; skip promoting durable knowledge into the knowledge base")
	pkbFeedCmd.Flags().BoolVar(&pkbFeedWriteDaily, "write-daily", false, "(re)build the daily feed md from scored articles; off by default because the news brief now owns vault/资讯/<date>.md")
	pkbFeedCmd.Flags().StringVar(&pkbCfgDir, "pkb-config", "config/pkb", "directory holding domains.yaml + prompts/")
	pkbCurateCmd.AddCommand(pkbFeedCmd)

	pkbProposalsCmd := &cobra.Command{
		Use:   "proposals <list|approve|reject> [id]",
		Short: "List, approve, or reject pending skeleton-change proposals (transitional approval gate)",
		Long: `proposals manages pending large-impact skeleton changes:
  proposals list            — list all pending proposals
  proposals approve <id>    — apply a proposal (snapshot + replace knowledge tree)
  proposals reject <id>     — discard a proposal (skeleton unchanged)
These are the CLI mirror of the Matrix !pkb approve/reject commands.`,
		Args: cobra.MinimumNArgs(1),
		Run:  runPkbProposals,
	}
	pkbCurateCmd.AddCommand(pkbProposalsCmd)

	pkbEvalCmd := &cobra.Command{
		Use:   "eval",
		Short: "Run golden-set scoring regression against the PKB score model",
		Long: `eval loads config/pkb/eval/*.json golden samples, scores each via the PKB
score model, and reports per-case diffs + overall accuracy + per-dimension MAE.
It is read-only: no files are moved, written, or deleted.
Use --json for machine-readable output. --tolerance sets per-dimension score
diff tolerance (default 2).`,
		Run: runPkbEval,
	}
	pkbEvalCmd.Flags().BoolVar(&pkbEvalJSON, "json", false, "output JSON for machine parsing")
	pkbEvalCmd.Flags().IntVar(&pkbEvalTolerance, "tolerance", 2, "per-dimension score diff tolerance")
	pkbEvalCmd.Flags().StringVar(&pkbCfgDir, "pkb-config", "config/pkb", "directory holding domains.yaml + prompts/ + eval/")
	pkbCurateCmd.AddCommand(pkbEvalCmd)

	rootCmd.AddCommand(serveCmd, versionCmd, migrateCmd, pkbCurateCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func runPkbDigest(cmd *cobra.Command, args []string) {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	if err := middleware.InitLogger(cfg.Logging.Level); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	db, err := model.InitDB(cfg.Database)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to database: %v\n", err)
		os.Exit(1)
	}
	articleRepo := repository.NewArticleTagRepository(db)
	var llmJobs *llmgateway.LLMJobQueueService
	if cfg.LLMJobQueue.Enabled {
		llmJobRepo := repository.NewLLMJobRepository(db)
		llmJobs = llmgateway.NewLLMJobQueueService(cfg.LLMJobQueue, llmJobRepo, llmclient.New(llmclient.Options{BaseURL: cfg.Classify.LLMProxyURL, APIKey: cfg.Server.APIKey, Timeout: 10 * time.Minute}), nil)
	}

	curator, err := pkb.NewCurator(cfg, pkb.Options{
		ConfigDir: pkbCfgDir,
		DryRun:    pkbDryRun,
		LLMJobs:   llmJobs,
	}, articleRepo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to init pkb curator: %v\n", err)
		os.Exit(1)
	}

	if err := curator.RunDigest(pkb.DigestOptions{
		Domain:   pkbDigestDomain,
		Period:   pkbDigestPeriod,
		Since:    pkbDigestSince,
		MaxCards: pkbDigestMaxCards,
		DryRun:   pkbDryRun,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "pkb-curate digest failed: %v\n", err)
		os.Exit(1)
	}
}

func runServer(cmd *cobra.Command, args []string) {
	a, err := app.NewApp(cfgFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize app: %v\n", err)
		os.Exit(1)
	}

	if err := a.Setup(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to setup application: %v\n", err)
		os.Exit(1)
	}

	if err := a.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}

func runMigrate(cmd *cobra.Command, args []string) {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	if err := middleware.InitLogger(cfg.Logging.Level); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	db, err := model.InitDB(cfg.Database)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to database: %v\n", err)
		os.Exit(1)
	}

	if err := model.AutoMigrate(db); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to run migrations: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Database migrations completed successfully")
}

func runPkbCurate(cmd *cobra.Command, args []string) {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	if err := middleware.InitLogger(cfg.Logging.Level); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	db, err := model.InitDB(cfg.Database)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to database: %v\n", err)
		os.Exit(1)
	}
	articleRepo := repository.NewArticleTagRepository(db)
	var llmJobs *llmgateway.LLMJobQueueService
	if cfg.LLMJobQueue.Enabled {
		llmJobRepo := repository.NewLLMJobRepository(db)
		llmJobs = llmgateway.NewLLMJobQueueService(cfg.LLMJobQueue, llmJobRepo, llmclient.New(llmclient.Options{BaseURL: cfg.Classify.LLMProxyURL, APIKey: cfg.Server.APIKey, Timeout: 10 * time.Minute}), nil)
	}

	curator, err := pkb.NewCurator(cfg, pkb.Options{
		ConfigDir: pkbCfgDir,
		DryRun:    pkbDryRun,
		Rescan:    pkbRescan,
		PerRun:    pkbPerRun,
		LLMJobs:   llmJobs,
	}, articleRepo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to init pkb curator: %v\n", err)
		os.Exit(1)
	}

	if err := curator.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "pkb-curate run failed: %v\n", err)
		os.Exit(1)
	}
}

func runPkbAudit(cmd *cobra.Command, args []string) {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	if err := middleware.InitLogger(cfg.Logging.Level); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	curator, err := pkb.NewCurator(cfg, pkb.Options{
		ConfigDir: pkbCfgDir,
	}, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to init pkb curator: %v\n", err)
		os.Exit(1)
	}

	if err := curator.RunAudit(pkbAuditJSON); err != nil {
		fmt.Fprintf(os.Stderr, "pkb-curate audit failed: %v\n", err)
		os.Exit(1)
	}
}

func runPkbEval(cmd *cobra.Command, args []string) {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	if err := middleware.InitLogger(cfg.Logging.Level); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	report, err := pkb.RunEval(cfg, pkb.EvalOptions{
		ConfigDir: pkbCfgDir,
		Tolerance: pkbEvalTolerance,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "pkb-curate eval failed: %v\n", err)
		os.Exit(1)
	}

	if pkbEvalJSON {
		out, _ := json.MarshalIndent(report, "", "  ")
		fmt.Println(string(out))
		return
	}

	printEvalReport(report)
}

func printEvalReport(r *pkb.EvalReport) {
	fmt.Printf("=== PKB Eval Report ===\n")
	fmt.Printf("Total: %d  Passed: %d  Accuracy: %.1f%%\n\n", r.Total, r.Passed, r.Accuracy*100)
	for _, c := range r.Cases {
		mark := "✓"
		if !c.Passed {
			mark = "✗"
		}
		fmt.Printf("%s [%s] %s\n", mark, c.ActualDecision, c.Title)
		fmt.Printf("    final=%.1f decision=%s(exp %s) match=%v\n",
			c.FinalScore, c.ActualDecision, c.ExpectedDecision, c.DecisionMatch)
		fmt.Printf("    diff rel=%d depth=%d action=%d durable=%d novelty=%d atomic=%d\n",
			c.ScoreDiff.Relevance, c.ScoreDiff.Depth, c.ScoreDiff.Actionability,
			c.ScoreDiff.Durability, c.ScoreDiff.Novelty, c.ScoreDiff.AtomicPotential)
		fmt.Printf("    domain_match=%v content_type_match=%v\n", c.DomainMatch, c.ContentTypeMatch)
		if c.Errors != "" {
			fmt.Printf("    errors: %s\n", c.Errors)
		}
	}
	fmt.Printf("\nMAE: rel=%.2f depth=%.2f action=%.2f durable=%.2f novelty=%.2f atomic=%.2f\n",
		r.MAE["relevance"], r.MAE["depth"], r.MAE["actionability"],
		r.MAE["durability"], r.MAE["novelty"], r.MAE["atomic_potential"])
}

func runPkbSkeleton(cmd *cobra.Command, args []string) {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	if err := middleware.InitLogger(cfg.Logging.Level); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	db, err := model.InitDB(cfg.Database)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to database: %v\n", err)
		os.Exit(1)
	}
	articleRepo := repository.NewArticleTagRepository(db)
	var llmJobs *llmgateway.LLMJobQueueService
	if cfg.LLMJobQueue.Enabled {
		llmJobRepo := repository.NewLLMJobRepository(db)
		llmJobs = llmgateway.NewLLMJobQueueService(cfg.LLMJobQueue, llmJobRepo, llmclient.New(llmclient.Options{BaseURL: cfg.Classify.LLMProxyURL, APIKey: cfg.Server.APIKey, Timeout: 10 * time.Minute}), nil)
	}

	curator, err := pkb.NewCurator(cfg, pkb.Options{
		ConfigDir: pkbCfgDir,
		DryRun:    pkbDryRun,
		LLMJobs:   llmJobs,
	}, articleRepo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to init pkb curator: %v\n", err)
		os.Exit(1)
	}

	var toc string
	if pkbSkeletonTOCFile != "" {
		data, err := os.ReadFile(pkbSkeletonTOCFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to read --toc-file %s: %v\n", pkbSkeletonTOCFile, err)
			os.Exit(1)
		}
		toc = string(data)
	}

	if err := curator.RunSkeleton(pkb.SkeletonOptions{
		Domain: args[0],
		TOC:    toc,
		DryRun: pkbDryRun,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "pkb-curate skeleton failed: %v\n", err)
		os.Exit(1)
	}
}

func runPkbMatch(cmd *cobra.Command, args []string) {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	if err := middleware.InitLogger(cfg.Logging.Level); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	db, err := model.InitDB(cfg.Database)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to database: %v\n", err)
		os.Exit(1)
	}
	articleRepo := repository.NewArticleTagRepository(db)
	var llmJobs *llmgateway.LLMJobQueueService
	if cfg.LLMJobQueue.Enabled {
		llmJobRepo := repository.NewLLMJobRepository(db)
		llmJobs = llmgateway.NewLLMJobQueueService(cfg.LLMJobQueue, llmJobRepo, llmclient.New(llmclient.Options{BaseURL: cfg.Classify.LLMProxyURL, APIKey: cfg.Server.APIKey, Timeout: 10 * time.Minute}), nil)
	}

	curator, err := pkb.NewCurator(cfg, pkb.Options{
		ConfigDir: pkbCfgDir,
		DryRun:    pkbDryRun,
		LLMJobs:   llmJobs,
	}, articleRepo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to init pkb curator: %v\n", err)
		os.Exit(1)
	}

	if err := curator.RunMatch(pkb.MatchOptions{
		Domain: args[0],
		DryRun: pkbDryRun,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "pkb-curate match failed: %v\n", err)
		os.Exit(1)
	}
}

func runPkbPropose(cmd *cobra.Command, args []string) {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}
	if err := middleware.InitLogger(cfg.Logging.Level); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	db, err := model.InitDB(cfg.Database)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to database: %v\n", err)
		os.Exit(1)
	}
	articleRepo := repository.NewArticleTagRepository(db)
	var llmJobs *llmgateway.LLMJobQueueService
	if cfg.LLMJobQueue.Enabled {
		llmJobRepo := repository.NewLLMJobRepository(db)
		llmJobs = llmgateway.NewLLMJobQueueService(cfg.LLMJobQueue, llmJobRepo, llmclient.New(llmclient.Options{BaseURL: cfg.Classify.LLMProxyURL, APIKey: cfg.Server.APIKey, Timeout: 10 * time.Minute}), nil)
	}
	curator, err := pkb.NewCurator(cfg, pkb.Options{
		ConfigDir: pkbCfgDir,
		DryRun:    pkbDryRun,
		LLMJobs:   llmJobs,
	}, articleRepo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to init pkb curator: %v\n", err)
		os.Exit(1)
	}
	if err := curator.RunPropose(pkb.ProposeOptions{
		Domain: args[0],
		DryRun: pkbDryRun,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "pkb-curate propose failed: %v\n", err)
		os.Exit(1)
	}
}

func runPkbFill(cmd *cobra.Command, args []string) {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}
	if err := middleware.InitLogger(cfg.Logging.Level); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	db, err := model.InitDB(cfg.Database)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to database: %v\n", err)
		os.Exit(1)
	}
	articleRepo := repository.NewArticleTagRepository(db)
	domainRepo := repository.NewCrawlDomainProfileRepository(db) // G3 冷却让路查 next_allowed_at
	var llmJobs *llmgateway.LLMJobQueueService
	if cfg.LLMJobQueue.Enabled {
		llmJobRepo := repository.NewLLMJobRepository(db)
		llmJobs = llmgateway.NewLLMJobQueueService(cfg.LLMJobQueue, llmJobRepo, llmclient.New(llmclient.Options{BaseURL: cfg.Classify.LLMProxyURL, APIKey: cfg.Server.APIKey, Timeout: 10 * time.Minute}), nil)
	}
	curator, err := pkb.NewCurator(cfg, pkb.Options{
		ConfigDir:  pkbCfgDir,
		DryRun:     pkbDryRun,
		LLMJobs:    llmJobs,
		DomainRepo: domainRepo,
	}, articleRepo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to init pkb curator: %v\n", err)
		os.Exit(1)
	}
	if err := curator.RunGapFill(pkb.GapFillOptions{
		Domain: args[0],
		DryRun: pkbDryRun,
		PerRun: pkbFillPerRun,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "pkb-curate fill failed: %v\n", err)
		os.Exit(1)
	}
}

func runPkbFeed(cmd *cobra.Command, args []string) {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}
	if err := middleware.InitLogger(cfg.Logging.Level); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	db, err := model.InitDB(cfg.Database)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to database: %v\n", err)
		os.Exit(1)
	}
	articleRepo := repository.NewArticleTagRepository(db)
	domainRepo := repository.NewCrawlDomainProfileRepository(db) // 晋升复用 fillOneGap，V2 核实查 next_allowed_at 冷却让路
	var llmJobs *llmgateway.LLMJobQueueService
	if cfg.LLMJobQueue.Enabled {
		llmJobRepo := repository.NewLLMJobRepository(db)
		llmJobs = llmgateway.NewLLMJobQueueService(cfg.LLMJobQueue, llmJobRepo, llmclient.New(llmclient.Options{BaseURL: cfg.Classify.LLMProxyURL, APIKey: cfg.Server.APIKey, Timeout: 10 * time.Minute}), nil)
	}
	curator, err := pkb.NewCurator(cfg, pkb.Options{
		ConfigDir:  pkbCfgDir,
		DryRun:     pkbDryRun,
		LLMJobs:    llmJobs,
		DomainRepo: domainRepo,
	}, articleRepo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to init pkb curator: %v\n", err)
		os.Exit(1)
	}
	if err := curator.RunFeed(pkb.FeedOptions{
		Date:           pkbFeedDate,
		DryRun:         pkbDryRun,
		SkipPromote:    pkbFeedNoPromote,
		SkipDailyWrite: !pkbFeedWriteDaily,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "pkb-curate feed failed: %v\n", err)
		os.Exit(1)
	}
}

func runPkbProposals(cmd *cobra.Command, args []string) {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}
	basePath := cfg.Knowledge.BasePath
	switch args[0] {
	case "list":
		props, err := pkb.ListPendingProposals(basePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "list proposals failed: %v\n", err)
			os.Exit(1)
		}
		if len(props) == 0 {
			fmt.Println("无待批骨架提议")
			return
		}
		for _, p := range props {
			fmt.Printf("- %s [%s 影响半径=%d] %s — %s\n", p.ID, p.Action, p.ImpactRadius, p.Domain, p.Summary)
		}
	case "approve":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "用法: pkb-curate proposals approve <id>")
			os.Exit(1)
		}
		msg, err := pkb.ApplySkeletonProposal(basePath, args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "approve failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(msg)
	case "reject":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "用法: pkb-curate proposals reject <id>")
			os.Exit(1)
		}
		msg, err := pkb.RejectSkeletonProposal(basePath, args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "reject failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(msg)
	default:
		fmt.Fprintf(os.Stderr, "未知子动作 %q（支持 list|approve|reject）\n", args[0])
		os.Exit(1)
	}
}
