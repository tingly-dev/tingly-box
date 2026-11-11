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
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/tingly-dev/tingly-box/projects/internal/config"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
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
	serve := &cobra.Command{
		Use:   "serve",
		Short: "Start the tingly-project server",
		RunE:  func(*cobra.Command, []string) error { return runServe(cfg) },
	}
	for _, c := range []*cobra.Command{serve} {
		c.Flags().StringVar(&cfg.Host, "host", cfg.Host, "address to listen on (use 0.0.0.0 to expose on the network; there is no login)")
		c.Flags().IntVar(&cfg.Port, "port", cfg.Port, "port to listen on")
		c.Flags().StringVar(&cfg.DataDir, "data-dir", cfg.DataDir, "where the database lives ($TINGLY_PROJECT_HOME)")
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
	root.AddCommand(serve, swaggerCmd, versionCmd)
	return root
}

func runServe(cfg config.Config) error {
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
		Handler:           server.New(ctx, server.Deps{Store: store, Layout: project.Layout{Root: cfg.ProjectsDir()}}),
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
