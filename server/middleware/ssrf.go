package middleware

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

type SSRFConfig struct {
	Enabled       bool     `json:"enabled"`
	BlockPrivate  bool     `json:"block_private" config:",optional"`
	BlockLoopback bool     `json:"block_loopback" config:",optional"`
	BlockMetadata bool     `json:"block_metadata" config:",optional"`
	AllowedHosts  []string `json:"allowed_hosts" config:",optional"`
	AllowAll      bool     `json:"allow_all" config:",optional"`
}

var (
	privateRanges = []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"100.64.0.0/10",
	}

	loopbackRanges = []string{
		"127.0.0.0/8",
		"::1",
	}

	metadataRanges = []string{
		"169.254.169.254/32",
	}
)

type SafeHTTPClient struct {
	client  *http.Client
	checker *ssrfChecker
}

type ssrfChecker struct {
	privateRanges  []*net.IPNet
	loopbackRanges []*net.IPNet
	metadataRanges []*net.IPNet
	allowedHosts   map[string]bool
	allowAll       bool
}

func NewSafeHTTPClient(cfg SSRFConfig) *SafeHTTPClient {
	checker := &ssrfChecker{
		allowedHosts: make(map[string]bool),
		allowAll:     cfg.AllowAll,
	}
	for _, h := range cfg.AllowedHosts {
		checker.allowedHosts[strings.ToLower(h)] = true
	}
	if cfg.BlockPrivate {
		for _, cidr := range privateRanges {
			_, n, _ := net.ParseCIDR(cidr)
			checker.privateRanges = append(checker.privateRanges, n)
		}
	}
	if cfg.BlockLoopback {
		for _, cidr := range loopbackRanges {
			_, n, _ := net.ParseCIDR(cidr)
			checker.loopbackRanges = append(checker.loopbackRanges, n)
		}
	}
	if cfg.BlockMetadata {
		for _, cidr := range metadataRanges {
			_, n, _ := net.ParseCIDR(cidr)
			checker.metadataRanges = append(checker.metadataRanges, n)
		}
	}
	return &SafeHTTPClient{
		client: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout: 5 * time.Second,
				}).DialContext,
			},
		},
		checker: checker,
	}
}

func (c *SafeHTTPClient) DoURL(ctx context.Context, urlStr, method string, body io.Reader) (*http.Response, error) {
	parsed, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("ssrf: parse url: %w", err)
	}

	host := parsed.Hostname()
	if err := c.checker.validate(host); err != nil {
		return nil, err
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil, fmt.Errorf("ssrf: scheme %q not allowed", scheme)
	}

	cleanPath := filepath.Clean(parsed.Path)
	if strings.Contains(cleanPath, "..") {
		return nil, fmt.Errorf("ssrf: path traversal blocked")
	}

	safeURL := &url.URL{
		Scheme: scheme,
		Path:   cleanPath,
	}
	if p := parsed.Port(); p != "" {
		safeURL.Host = net.JoinHostPort(host, p)
	} else {
		safeURL.Host = host
	}

	req, err := http.NewRequestWithContext(ctx, method, safeURL.String(), body)
	if err != nil {
		return nil, fmt.Errorf("ssrf: new request: %w", err)
	}
	req.Host = host

	return c.client.Do(req)
}

func (c *ssrfChecker) validate(host string) error {
	if c.allowAll {
		return nil
	}
	lower := strings.ToLower(host)
	if c.allowedHosts[lower] {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil {
		var r net.Resolver
		addrs, err := r.LookupIPAddr(context.Background(), host)
		if err != nil {
			return fmt.Errorf("ssrf: cannot resolve host %s", host)
		}
		if len(addrs) > 0 {
			ip = addrs[0].IP
		}
	}
	if ip == nil {
		return nil
	}
	for _, cidr := range c.privateRanges {
		if cidr.Contains(ip) {
			return fmt.Errorf("ssrf: request to private IP %s blocked", ip)
		}
	}
	for _, cidr := range c.loopbackRanges {
		if cidr.Contains(ip) {
			return fmt.Errorf("ssrf: request to loopback IP %s blocked", ip)
		}
	}
	for _, cidr := range c.metadataRanges {
		if cidr.Contains(ip) {
			return fmt.Errorf("ssrf: request to metadata IP %s blocked", ip)
		}
	}
	return nil
}
