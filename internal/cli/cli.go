package cli

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/albz/otelma/internal/api"
	"github.com/albz/otelma/internal/backend"
	"github.com/albz/otelma/internal/backend/echo"
	"github.com/albz/otelma/internal/manager"
	"github.com/albz/otelma/internal/scheduler"
)

// v0.1MemoryBudgetBytes is the default unified memory ceiling: 24GB, the
// target Apple Silicon hardware's total memory. Not yet configurable via
// flag; that's a follow-up once real backends make it matter in practice.
const v0dot1MemoryBudgetBytes = 24 * 1 << 30

// Run dispatches os.Args[1:] to the appropriate subcommand and returns the
// process exit code.
func Run(args []string) int {
	if len(args) == 0 {
		printUsage()
		return 1
	}

	baseURL := DefaultBaseURL
	c := newClient(baseURL)

	var err error
	switch args[0] {
	case "pull":
		err = runPull(c, args[1:])
	case "ps":
		err = runPS(c, args[1:])
	case "run":
		err = runRun(c, args[1:])
	case "serve":
		err = runServe(args[1:])
	default:
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
	fmt.Fprintln(os.Stderr, `usage: otelma <command> [arguments]

commands:
  pull <name> <path>     register a local model file
  ps                      list known models and their state
  run <name> <prompt>     load (if needed) and run a prompt
  serve                   start the local runtime API server`)
}

func runPull(c *client, args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: otelma pull <name> <path>")
	}
	var out map[string]any
	if err := c.post("/api/pull", map[string]string{"name": args[0], "path": args[1]}, &out); err != nil {
		return err
	}
	fmt.Printf("pulled %s (state=%s)\n", out["name"], out["state"])
	return nil
}

func runPS(c *client, args []string) error {
	var out []map[string]any
	if err := c.get("/api/ps", &out); err != nil {
		return err
	}
	if len(out) == 0 {
		fmt.Println("no models registered")
		return nil
	}
	fmt.Printf("%-24s %-12s %s\n", "NAME", "STATE", "MEMORY")
	for _, m := range out {
		fmt.Printf("%-24v %-12v %v\n", m["name"], m["state"], m["memory_footprint_bytes"])
	}
	return nil
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

func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := fs.String("addr", "localhost:11535", "address to listen on")
	if err := fs.Parse(args); err != nil {
		return err
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	reg := manager.NewRegistry()
	budget := manager.NewBudget(v0dot1MemoryBudgetBytes)
	mgr := manager.NewManager(reg, budget, func() backend.InferenceBackend { return echo.New() })
	sched := scheduler.New(mgr)
	srv := api.New(mgr, sched, log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Info("otelma server starting", "addr", *addr, "memory_budget_bytes", v0dot1MemoryBudgetBytes)
	return srv.ListenAndServe(ctx, *addr)
}
