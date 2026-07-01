package middleware

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

type SSRFConfig struct {
	Enabled        bool     `json:"enabled"`
	BlockPrivate   bool     `json:"block_private" config:",optional"`
	BlockLoopback  bool     `json:"block_loopback" config:",optional"`
	BlockMetadata  bool     `json:"block_metadata" config:",optional"`
	AllowedHosts   []string `json:"allowed_hosts" config:",optional"`
	AllowAll       bool     `json:"allow_all" config:",optional"`
}

var privateRanges = []string{
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"100.64.0.0/10",
}

var loopbackRanges = []string{
	"127.0.0.0/8",
	"::1",
}

var metadataRanges = []string{
	"169.254.169.254/32",
}

// SafeHTTPClient wraps an HTTP client with SSRF protection.
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

// NewSafeHTTPClient creates an HTTP client protected against SSRF.
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

// Do performs an HTTP request with SSRF validation.
func (c *SafeHTTPClient) Do(req *http.Request) (*http.Response, error) {
	if err := c.checker.validate(req.URL.Hostname()); err != nil {
		return nil, err
	}
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
		addrs, err := net.LookupIP(host)
		if err != nil {
			return fmt.Errorf("ssrf: cannot resolve host %s", host)
		}
		if len(addrs) > 0 {
			ip = addrs[0]
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
