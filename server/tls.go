package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/natuleadan/sdk-api/infra/logx"
	"golang.org/x/crypto/acme/autocert"
)

type TLSConfig struct {
	Enabled      bool         `json:"enabled"`
	Manual       *ManualTLS   `json:"manual" config:",optional"`
	Autocert     *AutocertTLS `json:"autocert" config:",optional"`
	MinVersion   string       `json:"min_version" config:",optional"`
	MaxVersion   string       `json:"max_version" config:",optional"`
	CurvePrefs   []string     `json:"curve_preferences" config:",optional"`
	CipherSuites []string     `json:"cipher_suites" config:",optional"`
	RedirectHTTP bool         `json:"redirect_http" config:",optional"`
	RedirectPort int          `json:"redirect_port" config:",optional"`
}

type ManualTLS struct {
	CertFile string `json:"cert_file"`
	KeyFile  string `json:"key_file"`
}

type AutocertTLS struct {
	Domains  []string `json:"domains"`
	Email    string   `json:"email"`
	CacheDir string   `json:"cache_dir" config:",optional"`
	Staging  bool     `json:"staging" config:",optional"`
}

func (s *Server) listenTLS() error {
	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)
	tlsCfg := s.config.TLS

	if tlsCfg == nil || !tlsCfg.Enabled {
		logx.Infof("server listening on %s (HTTP)", addr)
		return s.app.Listen(addr, fiber.ListenConfig{EnablePrefork: s.config.Prefork})
	}

	if tlsCfg.Manual != nil && tlsCfg.Manual.CertFile != "" {
		if tlsCfg.RedirectHTTP {
			go startRedirectServer(tlsCfg.RedirectPort, s.config.Prefork)
		}
		logx.Infof("server listening on %s (HTTPS manual)", addr)
		return s.app.Listen(addr, fiber.ListenConfig{
			CertFile:     tlsCfg.Manual.CertFile,
			CertKeyFile:  tlsCfg.Manual.KeyFile,
			EnablePrefork: s.config.Prefork,
		})
	}

	if tlsCfg.Autocert != nil && len(tlsCfg.Autocert.Domains) > 0 {
		m := &autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(tlsCfg.Autocert.Domains...),
			Email:      tlsCfg.Autocert.Email,
		}
		if tlsCfg.Autocert.CacheDir != "" {
			m.Cache = autocert.DirCache(tlsCfg.Autocert.CacheDir)
		}
		tlsCfgOut := m.TLSConfig()
		applyTLSConfig(tlsCfgOut, tlsCfg)

		var lc net.ListenConfig
		ln, err := lc.Listen(context.Background(), "tcp", addr)
		if err != nil {
			return fmt.Errorf("tls listen: %w", err)
		}
		tlsLn := tls.NewListener(ln, tlsCfgOut)

		if tlsCfg.RedirectHTTP {
			go startRedirectServer(tlsCfg.RedirectPort, s.config.Prefork)
		}
		logx.Infof("server listening on %s (HTTPS autocert)", addr)
		return s.app.Listener(tlsLn)
	}

	// Enabled but no config → fallback to HTTP
	logx.Infof("server listening on %s (HTTP)", addr)
	return s.app.Listen(addr, fiber.ListenConfig{EnablePrefork: s.config.Prefork})
}

func applyTLSConfig(tlsCfg *tls.Config, cfg *TLSConfig) {
	if cfg.MinVersion != "" {
		tlsCfg.MinVersion = parseTLSVersion(cfg.MinVersion)
	}
	if cfg.MaxVersion != "" {
		tlsCfg.MaxVersion = parseTLSVersion(cfg.MaxVersion)
	}
	if len(cfg.CurvePrefs) > 0 {
		tlsCfg.CurvePreferences = parseCurves(cfg.CurvePrefs)
	}
	if len(cfg.CipherSuites) > 0 {
		tlsCfg.CipherSuites = parseCiphers(cfg.CipherSuites)
	}
}

func parseTLSVersion(v string) uint16 {
	switch strings.ToLower(v) {
	case "1.0":
		return tls.VersionTLS10
	case "1.1":
		return tls.VersionTLS11
	case "1.2":
		return tls.VersionTLS12
	case "1.3":
		return tls.VersionTLS13
	default:
		return tls.VersionTLS12
	}
}

func parseCurves(curves []string) []tls.CurveID {
	ids := make([]tls.CurveID, 0, len(curves))
	for _, c := range curves {
		switch strings.ToUpper(c) {
		case "X25519":
			ids = append(ids, tls.X25519)
		case "P-256":
			ids = append(ids, tls.CurveP256)
		case "P-384":
			ids = append(ids, tls.CurveP384)
		case "P-521":
			ids = append(ids, tls.CurveP521)
		}
	}
	return ids
}

func parseCiphers(ciphers []string) []uint16 {
	ids := make([]uint16, 0, len(ciphers))
	for _, c := range ciphers {
		switch c {
		case "TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384":
			ids = append(ids, tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384)
		case "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256":
			ids = append(ids, tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256)
		case "TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384":
			ids = append(ids, tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384)
		case "TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256":
			ids = append(ids, tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256)
		case "TLS_AES_256_GCM_SHA384":
			ids = append(ids, tls.TLS_AES_256_GCM_SHA384)
		case "TLS_AES_128_GCM_SHA256":
			ids = append(ids, tls.TLS_AES_128_GCM_SHA256)
		case "TLS_CHACHA20_POLY1305_SHA256":
			ids = append(ids, tls.TLS_CHACHA20_POLY1305_SHA256)
		}
	}
	return ids
}

func startRedirectServer(port int, prefork bool) {
	if port <= 0 {
		port = 80
	}
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		host := c.Hostname()
		target := fmt.Sprintf("https://%s%s", host, c.OriginalURL())
		return c.Redirect().Status(308).To(target)
	})
	addr := fmt.Sprintf(":%d", port)
	logx.Infof("HTTP→HTTPS redirect server on %s", addr)
	if err := app.Listen(addr, fiber.ListenConfig{EnablePrefork: prefork}); err != nil {
		logx.Errorf("redirect server error: %v", err)
	}
}
