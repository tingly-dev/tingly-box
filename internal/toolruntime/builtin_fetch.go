package toolruntime

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-shiori/go-readability"
)

const (
	defaultMaxFetchSize = 1 * 1024 * 1024
	defaultFetchTimeout = 30 * time.Second
	maxRedirectHops     = 5
)

type builtinFetchResult struct {
	Tool        string `json:"tool"`
	URL         string `json:"url"`
	FinalURL    string `json:"final_url"`
	StatusCode  int    `json:"status_code"`
	ContentType string `json:"content_type"`
	Truncated   bool   `json:"truncated"`
	Content     string `json:"content"`
}

func (s *builtinSource) fetchURL(targetURL string) (*builtinFetchResult, error) {
	cacheKey := fetchCacheKey(targetURL)
	if cached, found := s.cache.Get(cacheKey); found {
		if result, ok := cached.(*builtinFetchResult); ok {
			return result, nil
		}
	}

	client, err := newBuiltinFetchClient(s.config)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; TinglyBox/1.0)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml,text/plain")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch returned status %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if !isAllowedFetchContentType(contentType) {
		return nil, fmt.Errorf("unsupported content type: %s", contentType)
	}

	maxFetchSize := int64(defaultMaxFetchSize)
	if s.config.MaxFetchSize > 0 {
		maxFetchSize = s.config.MaxFetchSize
	}
	limitedBody, truncated, err := readLimitedBody(resp.Body, maxFetchSize)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	content, err := extractReadableContent(string(limitedBody), resp.Request.URL.String(), contentType)
	if err != nil {
		return nil, err
	}

	result := &builtinFetchResult{
		Tool:        BuiltinToolFetch,
		URL:         targetURL,
		FinalURL:    resp.Request.URL.String(),
		StatusCode:  resp.StatusCode,
		ContentType: contentType,
		Truncated:   truncated,
		Content:     content,
	}
	s.cache.Set(cacheKey, result, "fetch")
	return result, nil
}

func newBuiltinFetchClient(config *builtinConfig) (*http.Client, error) {
	timeout := defaultFetchTimeout
	if config != nil && config.FetchTimeout > 0 {
		timeout = time.Duration(config.FetchTimeout) * time.Second
	}

	var proxyHost string
	var proxyPort string
	if config != nil && config.ProxyURL != "" {
		proxyURL, err := url.Parse(config.ProxyURL)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy URL: %w", err)
		}
		proxyHost = proxyURL.Hostname()
		proxyPort = proxyURL.Port()
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				host = addr
				port = ""
			}
			if !sameDialTarget(host, port, proxyHost, proxyPort) && isBlockedFetchHost(host) {
				return nil, fmt.Errorf("blocked hostname: %s", host)
			}
			if !sameDialTarget(host, port, proxyHost, proxyPort) {
				if err := blockResolvedPrivateAddrs(ctx, host); err != nil {
					return nil, err
				}
			}
			dialer := &net.Dialer{}
			target := host
			if port != "" {
				target = net.JoinHostPort(host, port)
			}
			return dialer.DialContext(ctx, network, target)
		},
	}

	if config != nil && config.ProxyURL != "" {
		proxyURL, _ := url.Parse(config.ProxyURL)
		transport.Proxy = http.ProxyURL(proxyURL)
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirectHops {
				return fmt.Errorf("too many redirects")
			}
			if err := validateBuiltinFetchURL(req.URL.String(), config); err != nil {
				return err
			}
			return nil
		},
	}, nil
}

func sameDialTarget(host, port, proxyHost, proxyPort string) bool {
	if proxyHost == "" {
		return false
	}
	if !strings.EqualFold(host, proxyHost) {
		return false
	}
	return proxyPort == "" || port == proxyPort
}

func blockResolvedPrivateAddrs(ctx context.Context, host string) error {
	addrs, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil
	}
	for _, addr := range addrs {
		if isBlockedFetchAddr(addr) {
			return fmt.Errorf("blocked resolved address: %s", addr.String())
		}
	}
	return nil
}

func readLimitedBody(body io.Reader, maxBytes int64) ([]byte, bool, error) {
	if maxBytes <= 0 {
		maxBytes = defaultMaxFetchSize
	}
	data, err := io.ReadAll(io.LimitReader(body, maxBytes+1))
	if err != nil {
		return nil, false, err
	}
	if int64(len(data)) > maxBytes {
		return data[:maxBytes], true, nil
	}
	return data, false, nil
}

func isAllowedFetchContentType(contentType string) bool {
	ct := strings.ToLower(contentType)
	return strings.Contains(ct, "text/html") ||
		strings.Contains(ct, "text/plain") ||
		strings.Contains(ct, "application/xhtml+xml")
}

func extractReadableContent(raw string, finalURL string, contentType string) (string, error) {
	if strings.Contains(strings.ToLower(contentType), "text/plain") {
		return strings.TrimSpace(raw), nil
	}
	parsedURL, err := url.Parse(finalURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse final URL: %w", err)
	}
	article, err := readability.FromReader(strings.NewReader(raw), parsedURL)
	if err == nil && strings.TrimSpace(article.TextContent) != "" {
		return strings.TrimSpace(article.TextContent), nil
	}
	return strings.TrimSpace(raw), nil
}
