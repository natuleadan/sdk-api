package middleware

import (
	"crypto/rand"
	"encoding/base64"
	"strings"
)

// CSPLevel defines pre-built CSP policies.
type CSPLevel string

const (
	CSPLevelBasic  CSPLevel = "basic"
	CSPLevelStrict CSPLevel = "strict"
)

// CSPConfig configures Content-Security-Policy generation.
type CSPConfig struct {
	Level              CSPLevel `json:"level" config:",default=basic"`
	DefaultSrc         []string `json:"default_src" config:",optional"`
	ScriptSrc          []string `json:"script_src" config:",optional"`
	StyleSrc           []string `json:"style_src" config:",optional"`
	ImgSrc             []string `json:"img_src" config:",optional"`
	ConnectSrc         []string `json:"connect_src" config:",optional"`
	FontSrc            []string `json:"font_src" config:",optional"`
	FrameSrc           []string `json:"frame_src" config:",optional"`
	FrameAncestors     []string `json:"frame_ancestors" config:",optional"`
	ObjectSrc          []string `json:"object_src" config:",optional"`
	BaseURI            []string `json:"base_uri" config:",optional"`
	FormAction         []string `json:"form_action" config:",optional"`
	UpgradeInsecureReq bool     `json:"upgrade_insecure_requests" config:",optional"`
}

// BuildCSP generates a Content-Security-Policy string from config.
func BuildCSP(cfg CSPConfig) string {
	if cfg.Level == CSPLevelStrict {
		return buildStrictCSP()
	}
	return buildCustomCSP(cfg)
}

func buildStrictCSP() string {
	return "default-src 'none'; script-src 'strict-dynamic' 'unsafe-inline' https:; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; font-src 'self'; connect-src 'self'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'; upgrade-insecure-requests"
}

func buildCustomCSP(cfg CSPConfig) string {
	var policy string
	policy += joinDirective("default-src", cfg.DefaultSrc, "'self'")
	policy += joinDirective("script-src", cfg.ScriptSrc, "'self'")
	policy += joinDirective("style-src", cfg.StyleSrc, "'self' 'unsafe-inline'")
	policy += joinDirective("img-src", cfg.ImgSrc, "'self' data: https:")
	policy += joinDirective("connect-src", cfg.ConnectSrc, "'self'")
	policy += joinDirective("font-src", cfg.FontSrc, "'self'")
	policy += joinDirective("frame-src", cfg.FrameSrc, "'none'")
	policy += joinDirective("frame-ancestors", cfg.FrameAncestors, "'none'")
	policy += joinDirective("object-src", cfg.ObjectSrc, "'none'")
	policy += joinDirective("base-uri", cfg.BaseURI, "'self'")
	policy += joinDirective("form-action", cfg.FormAction, "'self'")
	if cfg.UpgradeInsecureReq {
		policy += "upgrade-insecure-requests; "
	}
	if len(policy) > 2 {
		policy = policy[:len(policy)-2]
	}
	return policy
}

func joinDirective(name string, values []string, defaults string) string {
	if len(values) == 0 {
		if defaults == "" {
			return ""
		}
		return name + " " + defaults + "; "
	}
	var joined strings.Builder
	joined.WriteString(name)
	for _, v := range values {
		joined.WriteString(" " + v)
	}
	return joined.String() + "; "
}

// GenerateNonce creates a CSP nonce (base64 random 32 bytes).
func GenerateNonce() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
