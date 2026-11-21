// Command tingly-project runs the Projects web app: `tingly-project` (or
// `tingly-project serve`) starts the server; `tingly-project swagger` writes
// openapi.json for frontend codegen.
package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/tingly-dev/tingly-box/projects/internal/config"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
	"github.com/tingly-dev/tingly-box/projects/internal/harness/live"
	"github.com/tingly-dev/tingly-box/projects/internal/project"
	"github.com/tingly-dev/tingly-box/projects/internal/server"
)

func main() {
	if err := rootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	cfg := config.Config{Host: "127.0.0.1", Port: config.DefaultPort, DataDir: config.DefaultDataDir()}
	var sessions project.SessionConfig
	serve := &cobra.Command{
		Use:   "serve",
		Short: "Start the tingly-project server",
		RunE:  func(*cobra.Command, []string) error { return runServe(cfg, sessions) },
	}
	for _, c := range []*cobra.Command{serve} {
		c.Flags().StringVar(&cfg.Host, "host", cfg.Host, "address to listen on (use 0.0.0.0 to expose on the network; there is no login)")
		c.Flags().IntVar(&cfg.Port, "port", cfg.Port, "port to listen on")
		c.Flags().StringVar(&cfg.DataDir, "data-dir", cfg.DataDir, "where the database lives ($TINGLY_PROJECT_HOME)")
		c.Flags().DurationVar(&sessions.ThreadIdleTimeout, "thread-idle-timeout", 0, "close an idle thread's Claude Code process after this long; its next turn resumes the session (0: "+project.DefaultThreadIdleTimeout.String()+", negative: never)")
		c.Flags().DurationVar(&sessions.CoordinatorIdleTimeout, "coordinator-idle-timeout", 0, "close an idle coordinator's process after this long without any thread working (0: "+project.DefaultCoordinatorIdleTimeout.String()+", negative: never)")
	}

	out := "openapi.json"
	swaggerCmd := &cobra.Command{
		Use:   "swagger",
		Short: "Write the OpenAPI v3 document used by frontend codegen",
		RunE: func(*cobra.Command, []string) error {
			store, err := db.Open(":memory:")
			if err != nil {
				return err
			}
			defer store.Close()
			spec, err := server.OpenAPI(server.Deps{Store: store, Layout: project.Layout{Root: os.TempDir()}})
			if err != nil {
				return err
			}
			if err := os.WriteFile(out, []byte(spec+"\n"), 0o644); err != nil {
				return err
			}
			fmt.Println("wrote", out)
			return nil
		},
	}
	swaggerCmd.Flags().StringVarP(&out, "output", "o", out, "output file")

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Run:   func(*cobra.Command, []string) { fmt.Println(server.Version) },
	}

	root := &cobra.Command{
		Use:           "tingly-project",
		Short:         "Claude-style Projects: a coordinator conversation running Claude Code threads, on top of Tingly Box",
		SilenceUsage:  true,
		SilenceErrors: false,
		RunE:          serve.RunE,
	}
	root.Flags().AddFlagSet(serve.Flags())
	root.AddCommand(serve, swaggerCmd, versionCmd, harnessCmd())
	return root
}

// harnessCmd runs Projects journeys for real: this binary as the server, the
// real Claude Code CLI, and a scripted model served by `tingly-box vmodel`.
func harnessCmd() *cobra.Command {
	var o live.Options
	run := &cobra.Command{
		Use:   "run <journey|file>...",
		Short: "Run journeys against the real server and Claude Code, with a scripted model",
		Long: "Each journey starts its own tingly-project server (this binary) and `tingly-box vmodel` serving the journey's script, " +
			"in a fresh work directory with its own HOME and Claude Code config; your ~/.tingly-box and ~/.claude are not used. " +
			"It needs claude and git on PATH. A journey is a builtin name (see `harness list`) or a YAML file.",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if o.Projects == "" {
				self, err := os.Executable()
				if err != nil {
					return err
				}
				o.Projects = self
			}
			tb, err := exec.LookPath(o.TB)
			if err != nil {
				return fmt.Errorf("tingly-box binary %q not found: pass --tb", o.TB)
			}
			o.TB = tb
			if _, err := exec.LookPath("claude"); err != nil {
				return fmt.Errorf("the claude CLI is not on PATH")
			}
			o.Out = cmd.OutOrStdout()
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			var failed []string
			for _, arg := range args {
				j, err := live.Load(arg)
				if err != nil {
					return err
				}
				opts := o
				if o.WorkDir != "" && len(args) > 1 {
					opts.WorkDir = filepath.Join(o.WorkDir, j.Name)
				}
				res, err := live.Run(ctx, j, opts)
				if err != nil {
					return fmt.Errorf("journey %s: %w", j.Name, err)
				}
				if res.OK() {
					fmt.Fprintf(o.Out, "PASS %s\n", j.Name)
					continue
				}
				failed = append(failed, j.Name)
				fmt.Fprintf(o.Out, "FAIL %s (kept %s)\n", j.Name, res.WorkDir)
				for _, f := range res.Failures {
					fmt.Fprintf(o.Out, "  - %s\n", f)
				}
			}
			if len(failed) > 0 {
				return fmt.Errorf("%d of %d journeys failed: %s", len(failed), len(args), strings.Join(failed, ", "))
			}
			return nil
		},
	}
	run.Flags().StringVar(&o.TB, "tb", "tingly-box", "the tingly-box binary that serves the scripted model (`tingly-box vmodel`, with kind: script)")
	run.Flags().StringVar(&o.Projects, "projects-bin", "", "the tingly-project binary to run as the server (default: this one)")
	run.Flags().StringVar(&o.WorkDir, "work-dir", "", "where the run lives; pick a path whose parents hold no CLAUDE.md or .claude (default: a new temp directory)")
	run.Flags().BoolVar(&o.Keep, "keep", false, "keep the work directory of a passing journey (a failing one is always kept)")

	list := &cobra.Command{
		Use:   "list",
		Short: "List the builtin journeys",
		RunE: func(cmd *cobra.Command, _ []string) error {
			for _, name := range live.Builtin() {
				j, err := live.Load(name)
				if err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", name, strings.TrimSpace(j.Description))
			}
			return nil
		},
	}
	h := &cobra.Command{Use: "harness", Short: "Run Projects journeys against the real Claude Code CLI with a scripted model"}
	h.AddCommand(run, list)
	return h
}

func runServe(cfg config.Config, sessions project.SessionConfig) error {
	dataDir, err := filepath.Abs(cfg.DataDir)
	if err != nil {
		return err
	}
	cfg.DataDir = dataDir
	store, err := db.Open(cfg.DBPath(), db.WithProjectsDir(cfg.ProjectsDir()))
	if err != nil {
		return err
	}
	defer store.Close()

	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	srv := &http.Server{
		Addr:              addr,
		Handler:           server.New(ctx, server.Deps{Store: store, Layout: project.Layout{Root: cfg.ProjectsDir()}, Sessions: sessions}),
		ReadHeaderTimeout: 10 * time.Second,
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	logrus.Infof("tingly-project %s on http://%s (data: %s)", server.Version, addr, cfg.DataDir)

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-sig:
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
	return nil
}
