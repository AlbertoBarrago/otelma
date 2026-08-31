package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/albz/otelma/internal/api"
	"github.com/albz/otelma/internal/backend"
	"github.com/albz/otelma/internal/backend/echo"
	"github.com/albz/otelma/internal/backend/llamacpp"
	"github.com/albz/otelma/internal/catalog"
	"github.com/albz/otelma/internal/config"
	"github.com/albz/otelma/internal/manager"
	"github.com/albz/otelma/internal/scheduler"
)

// Run dispatches os.Args[1:] to the appropriate subcommand and returns the
// process exit code.
func Run(args []string) int {
	if len(args) == 0 {
		printUsage()
		return 1
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "otelma: warning: using defaults,", err)
		cfg = config.Default()
	}

	c := newClient(cfg.ClientBaseURL)

	switch args[0] {
	case "pull":
		err = runPull(c, args[1:])
	case "ps":
		err = runPS(c, args[1:])
	case "run":
		err = runRun(c, args[1:])
	case "serve":
		err = runServe(cfg, args[1:])
	case "list":
		err = runList(args[1:])
	case "config":
		err = runConfig(cfg, args[1:])
	case "help", "-h", "--help":
		printUsage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "otelma: unknown command %q\n\n", args[0])
		printUsage()
		return 1
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "otelma:", err)
		return 1
	}
	return 0
}

func printUsage() {
	printLogo()
	fmt.Println()
	fmt.Println(`usage: otelma <command> [arguments]

commands:
  pull <name> <source>    register a model: <source> is a local file path
                          or a Hugging Face reference "hf:<user>/<repo>[:quant]"
  list                     show curated Hugging Face models known to fit the
                          local memory budget, ready to use with pull
  ps                       list known models and their state
  run <name> <prompt>      load (if needed) and run a prompt
  serve                    start the local runtime API server
  config path|init|show    locate, scaffold, or print the config file
  help                     show this message

run 'otelma <command> -h' for flags on commands that support them`)
}

func runPull(c *client, args []string) error {
	if len(args) != 2 {
		return fmt.Errorf(`usage: otelma pull <name> <source>
  <source> is a local file path or "hf:<user>/<repo>[:quant]" (see otelma list)`)
	}
	name, source := args[0], args[1]
	if strings.HasPrefix(source, "hf:") {
		fmt.Printf("downloading %s from Hugging Face, this can take a while for larger models...\n", source)
	}
	var out map[string]any
	if err := c.post("/api/pull", map[string]string{"name": name, "path": source}, &out); err != nil {
		return err
	}
	fmt.Printf("pulled %s (state=%s)\n", out["name"], out["state"])
	return nil
}

func runList(args []string) error {
	fmt.Printf("%-16s %-8s %s\n", "NAME", "SIZE", "SOURCE")
	for _, e := range catalog.Entries {
		fmt.Printf("%-16s %-8s %s\n", e.Name, formatBytes(e.ApproxBytes), e.HFRef)
		fmt.Printf("%-16s %-8s %s\n", "", "", e.Description)
	}
	fmt.Println("\npull one with: otelma pull <local-name> <SOURCE column above>")
	return nil
}

type psModel struct {
	Name                 string `json:"name"`
	State                string `json:"state"`
	MemoryFootprintBytes uint64 `json:"memory_footprint_bytes"`
}

func runPS(c *client, args []string) error {
	var out []psModel
	if err := c.get("/api/ps", &out); err != nil {
		return err
	}
	if len(out) == 0 {
		fmt.Println("no models registered")
		return nil
	}
	fmt.Printf("%-24s %-12s %s\n", "NAME", "STATE", "MEMORY")
	for _, m := range out {
		fmt.Printf("%-24s %-12s %s\n", m.Name, m.State, formatBytes(m.MemoryFootprintBytes))
	}
	return nil
}

func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%dB", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

func runRun(c *client, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: otelma run <name> <prompt>")
	}
	name, prompt := args[0], args[1]
	var out struct {
		Output string `json:"output"`
	}
	if err := c.post("/api/run", map[string]string{"name": name, "prompt": prompt}, &out); err != nil {
		return err
	}
	fmt.Println(out.Output)
	return nil
}

func runServe(cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := fs.String("addr", cfg.ServeAddr, "address to listen on")
	backendName := fs.String("backend", cfg.Backend, "inference backend: llamacpp or echo")
	memoryBudgetBytes := fs.Uint64("memory-budget-bytes", cfg.MemoryBudgetBytes, "unified memory ceiling in bytes")
	if err := fs.Parse(args); err != nil {
		return err
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	llamaTimeout := time.Duration(cfg.LlamaCppStartupTimeoutSeconds) * time.Second
	var newBackend func() backend.InferenceBackend
	switch *backendName {
	case "llamacpp":
		newBackend = func() backend.InferenceBackend { return llamacpp.New(llamaTimeout) }
	case "echo":
		newBackend = func() backend.InferenceBackend { return echo.New() }
	default:
		return fmt.Errorf("unknown backend %q (want llamacpp or echo)", *backendName)
	}

	reg := manager.NewRegistry()
	budget := manager.NewBudget(*memoryBudgetBytes)
	mgr := manager.NewManager(reg, budget, newBackend)
	mgr.HFDownloadTimeout = time.Duration(cfg.HuggingFaceDownloadTimeoutMinutes) * time.Minute
	sched := scheduler.New(mgr)
	srv := api.New(mgr, sched, log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Info("otelma server starting", "addr", *addr, "memory_budget_bytes", *memoryBudgetBytes, "backend", *backendName)
	return srv.ListenAndServe(ctx, *addr)
}

func runConfig(cfg config.Config, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: otelma config path|init|show")
	}

	switch args[0] {
	case "path":
		path, err := config.Path()
		if err != nil {
			return err
		}
		fmt.Println(path)
		return nil

	case "init":
		fs := flag.NewFlagSet("config init", flag.ContinueOnError)
		force := fs.Bool("force", false, "overwrite an existing config file")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		path, err := config.Init(config.Default(), *force)
		if err != nil {
			return err
		}
		fmt.Printf("wrote default config to %s\n", path)
		return nil

	case "show":
		data, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil

	default:
		return fmt.Errorf("usage: otelma config path|init|show")
	}
}
