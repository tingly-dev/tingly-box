package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/tingly-dev/tingly-box/projects/internal/harness/live"
)

// harnessCmd runs Projects journeys for real: this binary as the server, the
// real Claude Code CLI, and a scripted model served by `tingly-box vmodel`.
// Every run lives in its own directory (own HOME, Claude Code config,
// sockets, temp, database and worktrees) that can be opened in the web UI
// afterwards and removed with `harness clean`.
func harnessCmd() *cobra.Command {
	var o live.Options
	var openAfter bool
	var port int
	run := &cobra.Command{
		Use:   "run <journey|file>...",
		Short: "Run journeys against the real server and Claude Code, with a scripted model",
		Long: "Each journey starts its own tingly-project server (this binary) and `tingly-box vmodel` serving the journey's script, " +
			"in a fresh run directory under --root with its own HOME, Claude Code config, sockets and temp; your ~/.tingly-project, " +
			"~/.tingly-box and ~/.claude are neither read nor written. It needs claude and git on PATH. A journey is a builtin name " +
			"(see `harness list`) or a YAML file. With --open the last journey's project stays served for the web UI.",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := resolveBinaries(&o.Projects, &o.TB); err != nil {
				return err
			}
			if _, err := exec.LookPath("claude"); err != nil {
				return fmt.Errorf("the claude CLI is not on PATH")
			}
			o.Out = cmd.OutOrStdout()
			if openAfter {
				o.Keep = true
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			var failed []string
			var last string
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
				last = res.WorkDir
				if res.OK() {
					if o.Keep {
						fmt.Fprintf(o.Out, "PASS %s (kept %s)\n", j.Name, res.WorkDir)
					} else {
						fmt.Fprintf(o.Out, "PASS %s\n", j.Name)
					}
					continue
				}
				failed = append(failed, j.Name)
				fmt.Fprintf(o.Out, "FAIL %s (kept %s)\n", j.Name, res.WorkDir)
				for _, f := range res.Failures {
					fmt.Fprintf(o.Out, "  - %s\n", f)
				}
			}
			if openAfter && last != "" && ctx.Err() == nil {
				if err := live.Open(ctx, last, live.OpenOptions{Projects: o.Projects, TB: o.TB, Port: port, Out: o.Out}); err != nil {
					return err
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
	run.Flags().StringVar(&o.Root, "root", live.DefaultRoot(), "where run directories are made")
	run.Flags().StringVar(&o.WorkDir, "work-dir", "", "an exact, empty run directory instead of a new one under --root; its parents should hold no CLAUDE.md or .claude")
	run.Flags().BoolVar(&o.Keep, "keep", false, "keep the run directory of a passing journey (a failing one is always kept)")
	run.Flags().BoolVar(&openAfter, "open", false, "after the journeys, serve the last one's project for the web UI until Ctrl-C (implies --keep)")
	run.Flags().IntVar(&port, "port", 0, "with --open, the port to serve on (default: a free one)")

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

	var root string
	runs := &cobra.Command{
		Use:   "runs",
		Short: "List the kept run directories",
		RunE: func(cmd *cobra.Command, _ []string) error {
			rs, err := live.Runs(root)
			if err != nil {
				return err
			}
			if len(rs) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "no runs under %s\n", root)
			}
			for _, r := range rs {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", r.Meta.Started.Local().Format(time.DateTime), r.Meta.Journey, r.Dir)
			}
			return nil
		},
	}
	runs.Flags().StringVar(&root, "root", live.DefaultRoot(), "where run directories are made")

	var oo live.OpenOptions
	var noModel bool
	open := &cobra.Command{
		Use:   "open [run-dir]",
		Short: "Serve a kept run's project for the web UI, with the run's isolated environment",
		Long: "Starts this binary on the run directory's data (and `tingly-box vmodel` with a placeholder model, so messages sent " +
			"from the UI get an answer) and prints the project URL. Without an argument it opens the newest run under --root.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := resolveBinaries(&oo.Projects, &oo.TB); err != nil && !noModel {
				return err
			}
			if noModel {
				oo.TB = ""
			}
			dir := ""
			if len(args) == 1 {
				dir = args[0]
			} else {
				rs, err := live.Runs(root)
				if err != nil {
					return err
				}
				if len(rs) == 0 {
					return fmt.Errorf("no runs under %s", root)
				}
				dir = rs[0].Dir
			}
			oo.Out = cmd.OutOrStdout()
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return live.Open(ctx, dir, oo)
		},
	}
	open.Flags().StringVar(&oo.TB, "tb", "tingly-box", "the tingly-box binary for the placeholder model")
	open.Flags().StringVar(&oo.Projects, "projects-bin", "", "the tingly-project binary to serve with (default: this one)")
	open.Flags().IntVar(&oo.Port, "port", 0, "the port to serve on (default: a free one)")
	open.Flags().BoolVar(&noModel, "no-model", false, "do not start a placeholder model")
	open.Flags().StringVar(&root, "root", live.DefaultRoot(), "where run directories are made")

	var all bool
	clean := &cobra.Command{
		Use:   "clean [run-dir...]",
		Short: "Remove kept run directories (only directories the harness made)",
		RunE: func(cmd *cobra.Command, args []string) error {
			dirs := args
			if len(dirs) == 0 {
				if !all {
					return fmt.Errorf("name run directories, or pass --all to remove every run under %s", root)
				}
				rs, err := live.Runs(root)
				if err != nil {
					return err
				}
				for _, r := range rs {
					dirs = append(dirs, r.Dir)
				}
			}
			if err := live.Clean(dirs); err != nil {
				return err
			}
			for _, d := range dirs {
				fmt.Fprintf(cmd.OutOrStdout(), "removed %s\n", d)
			}
			return nil
		},
	}
	clean.Flags().BoolVar(&all, "all", false, "remove every run under --root")
	clean.Flags().StringVar(&root, "root", live.DefaultRoot(), "where run directories are made")

	h := &cobra.Command{Use: "harness", Short: "Run Projects journeys against the real Claude Code CLI with a scripted model"}
	h.AddCommand(run, list, runs, open, clean)
	return h
}

// resolveBinaries fills in this binary as the server and finds tingly-box.
func resolveBinaries(projects, tb *string) error {
	if *projects == "" {
		self, err := os.Executable()
		if err != nil {
			return err
		}
		*projects = self
	}
	path, err := exec.LookPath(*tb)
	if err != nil {
		return fmt.Errorf("tingly-box binary %q not found: pass --tb", *tb)
	}
	*tb = path
	return nil
}
