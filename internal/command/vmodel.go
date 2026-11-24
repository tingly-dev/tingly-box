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
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/vmodel/capture"
	"github.com/tingly-dev/tingly-box/vmodel/spec"
)

// VModelCmdKong starts an isolated provider without the gateway or its stores.
type VModelCmdKong struct {
	Config []string `kong:"name='config',help='JSON/YAML model definition or list (repeatable); omit to use builtin models'"`
	Host   string   `kong:"name='host',default='127.0.0.1',help='Address to bind'"`
	Port   int      `kong:"name='port',default='0',help='Port to listen on; defaults to a free port'"`
	JSON   bool     `kong:"name='json',help='Print one JSON readiness record with URLs and model IDs'"`
	Export string   `kong:"name='export',help='Export definitions to a .json or .yaml file and exit; omit --config to export the full builtin catalog'"`

	Fallback string `kong:"name='fallback',help='Serve request model ids that no definition names with this model ID instead of a 404; with --capture it defaults to a built-in mock'"`

	Capture          string `kong:"name='capture',help='Record what clients send into this new/empty directory: verbatim bytes of every connection (raw/conn-N.in) plus index.jsonl'"`
	CaptureOut       bool   `kong:"name='capture-out',default='true',negatable,help='With --capture, also record the bytes written back to clients (raw/conn-N.out)'"`
	CaptureRequestID bool   `kong:"name='capture-request-id',help='With --capture, add a request-id response header (req_vmodel_<seq>) as the real API does'"`
}

func (c *VModelCmdKong) Run() error {
	if c.Export != "" && c.Capture != "" {
		return fmt.Errorf("--export and --capture cannot be combined")
	}
	if c.Export != "" {
		defs, err := c.definitions(true)
		if err != nil {
			return err
		}
		data, err := spec.Export(defs, strings.TrimPrefix(filepath.Ext(c.Export), "."))
		if err != nil {
			return err
		}
		return os.WriteFile(c.Export, data, 0600)
	}
	if c.Port < 0 || c.Port > 65535 {
		return fmt.Errorf("port must be 0..65535")
	}
	if c.Capture == "" && c.CaptureRequestID {
		return fmt.Errorf("--capture-request-id requires --capture")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	gin.SetMode(gin.ReleaseMode)
	return c.run(ctx, os.Stdout)
}

type vmodelReady struct {
	URL              string   `json:"url"`
	OpenAIBaseURL    string   `json:"openai_base_url"`
	AnthropicBaseURL string   `json:"anthropic_base_url"`
	Models           []string `json:"models"`
	GoogleBaseURL    string   `json:"google_base_url,omitempty"`
	CaptureDir       string   `json:"capture_dir,omitempty"`
	// Scripts are the script definitions served; their control interface is
	// under <url>/_vmodel/scripts.
	Scripts []string `json:"scripts,omitempty"`
}

func (c *VModelCmdKong) definitions(all bool) ([]spec.Definition, error) {
	if len(c.Config) == 0 {
		if all {
			return spec.AllDefinitions(), nil
		}
		return spec.DefaultDefinitions(), nil
	}
	var defs []spec.Definition
	for _, path := range c.Config {
		loaded, err := spec.Load(path)
		if err != nil {
			return nil, err
		}
		defs = append(defs, loaded...)
	}
	return defs, nil
}
func (c *VModelCmdKong) handler() (*spec.Runtime, error) {
	defs, err := c.definitions(false)
	if err != nil {
		return nil, err
	}
	fallback := c.Fallback
	if fallback == "" && c.Capture != "" {
		// Capturing observes a client whose model ids we cannot know in advance;
		// it must still be answered so it finishes its round.
		fallback = captureDefaultModel
		if defs, err = withCaptureDefault(defs); err != nil {
			return nil, err
		}
	}
	runtime, err := spec.New(defs)
	if err != nil {
		return nil, err
	}
	if fallback != "" {
		if err := runtime.SetFallback(fallback); err != nil {
			return nil, err
		}
	}
	return runtime, nil
}

// captureDefaultModel answers unknown model ids while capturing, unless the
// user names another fallback.
const captureDefaultModel = "capture-default"

// withCaptureDefault adds that model unless the user already defined one under
// the same id.
func withCaptureDefault(defs []spec.Definition) ([]spec.Definition, error) {
	for _, d := range defs {
		if d.ID == captureDefaultModel {
			return defs, nil
		}
	}
	extra, err := spec.Decode([]byte("id: " + captureDefaultModel + "\nkind: mock\nmock:\n  content: \"ok\"\n"))
	if err != nil {
		return nil, err
	}
	return append(defs, extra...), nil
}

func (c *VModelCmdKong) run(ctx context.Context, out io.Writer) error {
	if c.Port < 0 || c.Port > 65535 {
		return fmt.Errorf("port must be 0..65535")
	}
	runtime, err := c.handler()
	if err != nil {
		return err
	}
	var handler http.Handler = runtime
	models := runtime.IDs()
	var tap *capture.Tap
	if c.Capture != "" {
		tap, err = capture.New(c.Capture, capture.Options{
			Out:       c.CaptureOut,
			RequestID: c.CaptureRequestID,
			Warn:      func(msg string) { fmt.Fprintln(os.Stderr, msg) },
		})
		if err != nil {
			return err
		}
		defer tap.Close()
		handler = tap.Handler(handler)
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(c.Host, strconv.Itoa(c.Port)))
	if err != nil {
		return fmt.Errorf("vmodel listen: %w", err)
	}
	defer listener.Close()
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}
	defer server.Close()
	serveOn := net.Listener(listener)
	if tap != nil {
		tap.Install(server)
		serveOn = tap.Listener(listener)
	}
	served := make(chan error, 1)
	go func() { served <- server.Serve(serveOn) }()
	address := listener.Addr().(*net.TCPAddr)
	host := address.IP.String()
	if address.IP.IsUnspecified() {
		host = "127.0.0.1"
		if address.IP.To4() == nil {
			host = "::1"
		}
	}
	base := "http://" + net.JoinHostPort(host, strconv.Itoa(address.Port))
	ready := vmodelReady{URL: base, OpenAIBaseURL: base + "/v1", AnthropicBaseURL: base, Models: models, GoogleBaseURL: base, Scripts: runtime.ScriptIDs()}
	if tap != nil {
		ready.CaptureDir = tap.Dir()
		if err := tap.SetMeta(map[string]any{"argv": os.Args, "url": base, "models": models,
			"capture_out": c.CaptureOut, "capture_request_id": c.CaptureRequestID}); err != nil {
			return fmt.Errorf("write capture meta: %w", err)
		}
	}

	if c.JSON {
		err = json.NewEncoder(out).Encode(ready)
	} else {
		_, err = fmt.Fprintf(out, "VModel ready: %s\nOpenAI base URL: %s\nAnthropic base URL: %s\nModels (request model IDs): %v\n", ready.URL, ready.OpenAIBaseURL, ready.AnthropicBaseURL, ready.Models)
		if err == nil && ready.CaptureDir != "" {
			_, err = fmt.Fprintf(out, "Capturing to: %s\n", ready.CaptureDir)
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
		return finishScripts(runtime, ready.CaptureDir, os.Stderr)
	}
}

// finishScripts ends every script run, prints its report and, when
// capturing, keeps report and trace next to the capture; a failed script
// fails the command.
func finishScripts(runtime *spec.Runtime, dir string, out io.Writer) error {
	var failed []string
	for _, id := range runtime.ScriptIDs() {
		engine := runtime.Scripts()[id]
		report := engine.Finish()
		fmt.Fprint(out, report.String())
		if dir != "" {
			data, _ := json.MarshalIndent(map[string]any{"report": report, "trace": engine.Trace()}, "", " ")
			if err := os.WriteFile(filepath.Join(dir, "script-"+id+".json"), append(data, '\n'), 0o644); err != nil {
				return fmt.Errorf("write script report: %w", err)
			}
		}
		if !report.OK {
			failed = append(failed, id)
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("script %s failed", strings.Join(failed, ", "))
	}
	return nil
}
