package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/app"
	"github.com/tingly-dev/tingly-box/vmodel"
	"github.com/tingly-dev/tingly-box/vmodel/anthropic"
	"github.com/tingly-dev/tingly-box/vmodel/openai"
	"github.com/tingly-dev/tingly-box/vmodel/virtualserver"
)

// VModelCmdKong starts an isolated provider without the gateway or its stores.
type VModelCmdKong struct {
	Config     []string `kong:"name='config',help='JSON/YAML mock model file (repeatable); omit to use builtin models'"`
	Host       string   `kong:"name='host',default='127.0.0.1',help='Address to bind'"`
	Port       int      `kong:"name='port',default='0',help='Port to listen on; defaults to a free port'"`
	JSON       bool     `kong:"name='json',help='Print one JSON readiness record with URLs and model IDs'"`
	Standalone bool     `kong:"name='standalone',help='Run without registering providers and rules in the running tb instance'"`
}

func (c *VModelCmdKong) Run(appManager *app.AppManager) error {
	if c.Port < 0 || c.Port > 65535 {
		return fmt.Errorf("port must be 0..65535")
	}
	var attach vmodelAttach
	if !c.Standalone {
		gateway, err := runningVModelGateway(appManager)
		if err != nil {
			return err
		}
		attach = gateway.attach
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	gin.SetMode(gin.ReleaseMode)
	return c.runAttached(ctx, os.Stdout, attach)
}

type vmodelReady struct {
	URL              string   `json:"url"`
	OpenAIBaseURL    string   `json:"openai_base_url"`
	AnthropicBaseURL string   `json:"anthropic_base_url"`
	Models           []string `json:"models"`
	GatewayBaseURL   string   `json:"gateway_base_url,omitempty"`
	Scenario         string   `json:"scenario,omitempty"`
}

func (c *VModelCmdKong) handler() (http.Handler, []string, error) {
	var svc *virtualserver.Service
	if len(c.Config) == 0 {
		svc = virtualserver.NewService()
	} else {
		svc = virtualserver.NewEmptyService()
		for _, path := range c.Config {
			spec, err := vmodel.LoadMockSpec(path)
			if err != nil {
				return nil, nil, err
			}
			if err := svc.GetOpenAIRegistry().Register(openai.NewMockModel(spec)); err != nil {
				return nil, nil, fmt.Errorf("%s: %w", path, err)
			}
			if err := svc.GetAnthropicRegistry().Register(anthropic.NewMockModel(spec)); err != nil {
				return nil, nil, fmt.Errorf("%s: %w", path, err)
			}
		}
	}
	router := gin.New()
	router.Use(gin.Recovery())
	router.NoRoute(svc.GetHandler().NotSupported)
	svc.SetupOpenAIRoutes(router.Group("/openai"))
	svc.SetupAnthropicRoutes(router.Group("/anthropic"))
	// Conventional /v1 entrypoints also allow clients to use the bare URL.
	svc.SetupOpenAIRoutes(router.Group(""))
	router.POST("/v1/messages", svc.GetHandler().Messages)
	router.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ready"}) })
	seen := make(map[string]bool)
	for _, m := range svc.GetOpenAIRegistry().ListModels() {
		seen[m.ID] = true
	}
	for _, m := range svc.GetAnthropicRegistry().ListModels() {
		seen[m.ID] = true
	}
	for _, m := range svc.GetDecisionRegistry().ListModels() {
		seen[m.ID] = true
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return router, ids, nil
}

func (c *VModelCmdKong) run(ctx context.Context, out io.Writer) error {
	return c.runAttached(ctx, out, nil)
}

type vmodelAttach func(context.Context, *vmodelReady) (func() error, error)

func (c *VModelCmdKong) runAttached(ctx context.Context, out io.Writer, attach vmodelAttach) (retErr error) {
	if c.Port < 0 || c.Port > 65535 {
		return fmt.Errorf("port must be 0..65535")
	}
	handler, models, err := c.handler()
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(c.Host, strconv.Itoa(c.Port)))
	if err != nil {
		return fmt.Errorf("vmodel listen: %w", err)
	}
	defer listener.Close()
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}
	defer server.Close()
	served := make(chan error, 1)
	go func() { served <- server.Serve(listener) }()
	address := listener.Addr().(*net.TCPAddr)
	host := address.IP.String()
	if address.IP.IsUnspecified() {
		host = "127.0.0.1"
		if address.IP.To4() == nil {
			host = "::1"
		}
	}
	base := "http://" + net.JoinHostPort(host, strconv.Itoa(address.Port))
	ready := vmodelReady{URL: base, OpenAIBaseURL: base + "/openai/v1", AnthropicBaseURL: base + "/anthropic", Models: models}
	if attach != nil {
		cleanup, err := attach(ctx, &ready)
		if err != nil {
			return fmt.Errorf("register vmodel in tb: %w", err)
		}
		defer func() { retErr = errors.Join(retErr, cleanup()) }()
	}
	if c.JSON {
		err = json.NewEncoder(out).Encode(ready)
	} else {
		_, err = fmt.Fprintf(out, "VModel ready: %s\nOpenAI base URL: %s\nAnthropic base URL: %s\nModels: %v\n", ready.URL, ready.OpenAIBaseURL, ready.AnthropicBaseURL, ready.Models)
		if err == nil && ready.GatewayBaseURL != "" {
			_, err = fmt.Fprintf(out, "tb scenario: %s\nGateway base URL: %s\n", ready.Scenario, ready.GatewayBaseURL)
		}
	}
	if err != nil {
		return fmt.Errorf("write vmodel readiness: %w", err)
	}
	select {
	case err := <-served:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			return fmt.Errorf("vmodel shutdown: %w", err)
		}
		return nil
	}
}
