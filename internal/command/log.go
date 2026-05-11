package command

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/tingly-dev/tingly-box/pkg/lock"
)

// LogCmdKong streams or fetches system logs from the running tingly-box server
// via its HTTP API (/api/v1/system/logs). Default mode is real-time follow.
type LogCmdKong struct {
	Once     bool          `kong:"flag,name='once',help='Fetch logs once and exit (default is real-time follow)'"`
	Limit    int           `kong:"flag,name='limit',short='n',default='50',help='Number of recent log entries per fetch (max 1000)'"`
	Interval time.Duration `kong:"flag,name='interval',short='i',default='2s',help='Poll interval for real-time follow mode'"`
	Level    string        `kong:"flag,name='level',help='Filter by minimum log level (debug|info|warn|error)'"`
	Host     string        `kong:"flag,name='host',default='localhost',help='Server host'"`
	Port     int           `kong:"flag,name='port',short='p',help='Server port (defaults to configured port)'"`
}

type logEntry struct {
	Time    time.Time              `json:"time"`
	Level   string                 `json:"level"`
	Message string                 `json:"message"`
	Fields  map[string]interface{} `json:"fields,omitempty"`
}

type logsResponse struct {
	Total int        `json:"total"`
	Logs  []logEntry `json:"logs"`
}

func (l *LogCmdKong) Run(appManager *AppManager) error {
	appConfig := appManager.AppConfig()

	// Make sure the server is actually running, otherwise the API is not reachable.
	fileLock := lock.NewFileLock(appConfig.ConfigDir())
	if !fileLock.IsLocked() {
		return fmt.Errorf("server is not running; start it first with 'tingly-box start'")
	}

	port := l.Port
	if port == 0 {
		port = appConfig.GetServerPort()
	}
	host := l.Host
	if host == "" {
		host = "localhost"
	}
	baseURL := fmt.Sprintf("http://%s:%d/api/v1/system/logs", host, port)

	token := appManager.GetUserToken()

	limit := l.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}

	levelFilter := strings.ToLower(strings.TrimSpace(l.Level))

	client := &http.Client{Timeout: 10 * time.Second}

	if l.Once {
		entries, err := fetchLogs(client, baseURL, token, limit)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if !levelAllowed(levelFilter, e.Level) {
				continue
			}
			printLogEntry(e)
		}
		return nil
	}

	// Real-time follow mode: poll the API and print only entries newer than the
	// last one we've seen so the user gets a continuous tail.
	interval := l.Interval
	if interval <= 0 {
		interval = 2 * time.Second
	}

	ctx, cancel := signalContext()
	defer cancel()

	fmt.Fprintf(os.Stderr, "Following system logs from %s (interval %s, Ctrl+C to stop)\n", baseURL, interval)

	var lastTime time.Time
	first := true
	for {
		entries, err := fetchLogs(client, baseURL, token, limit)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warn: %v\n", err)
		} else {
			for _, e := range entries {
				if !first && !e.Time.After(lastTime) {
					continue
				}
				if !levelAllowed(levelFilter, e.Level) {
					continue
				}
				printLogEntry(e)
				if e.Time.After(lastTime) {
					lastTime = e.Time
				}
			}
			first = false
		}

		select {
		case <-ctx.Done():
			fmt.Fprintln(os.Stderr, "\nStopped.")
			return nil
		case <-time.After(interval):
		}
	}
}

func signalContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		cancel()
	}()
	return ctx, cancel
}

func fetchLogs(client *http.Client, baseURL, token string, limit int) ([]logEntry, error) {
	url := fmt.Sprintf("%s?limit=%d", baseURL, limit)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call API: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("api returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed logsResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return parsed.Logs, nil
}

// levelAllowed returns true when entryLevel meets the minimum filter level.
// An empty filter means accept everything.
func levelAllowed(filter, entryLevel string) bool {
	if filter == "" {
		return true
	}
	return logLevelWeight(entryLevel) >= logLevelWeight(filter)
}

func logLevelWeight(level string) int {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "trace":
		return 0
	case "debug":
		return 1
	case "info":
		return 2
	case "warn", "warning":
		return 3
	case "error":
		return 4
	case "fatal":
		return 5
	case "panic":
		return 6
	}
	return 2
}

func printLogEntry(e logEntry) {
	ts := e.Time.Local().Format("2006-01-02 15:04:05.000")
	level := strings.ToUpper(e.Level)
	if level == "" {
		level = "INFO"
	}
	line := fmt.Sprintf("%s [%s] %s", ts, level, e.Message)
	if len(e.Fields) > 0 {
		extras := make([]string, 0, len(e.Fields))
		for k, v := range e.Fields {
			extras = append(extras, fmt.Sprintf("%s=%v", k, v))
		}
		line += "  " + strings.Join(extras, " ")
	}
	fmt.Println(line)
}
