package middleware

import (
	"github.com/gofiber/fiber/v2"
)

type SecurityHeadersConfig struct {
	FrameOptions      string   `json:"frame_options" config:",optional"`
	ReferrerPolicy    string   `json:"referrer_policy" config:",optional"`
	PermissionsPolicy string   `json:"permissions_policy" config:",optional"`
	HSTS              bool     `json:"hsts" config:",optional"`
	HSTSMaxAge        int      `json:"hsts_max_age" config:",optional"`
	HSTSIncludeSubs   bool     `json:"hsts_include_subdomains" config:",optional"`
	CSP               string   `json:"csp" config:",optional"`
	COOP              string   `json:"coop" config:",optional"`
	COEP              string   `json:"coep" config:",optional"`
	CORP              string   `json:"corp" config:",optional"`
	CacheControl      string   `json:"cache_control" config:",optional"`
	CSPReportPath     string   `json:"csp_report_path" config:",optional"`
}

func SecurityHeaders(cfg SecurityHeadersConfig) fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Set("X-Content-Type-Options", "nosniff")

		if cfg.FrameOptions != "" {
			c.Set("X-Frame-Options", cfg.FrameOptions)
		}
		if cfg.ReferrerPolicy != "" {
			c.Set("Referrer-Policy", cfg.ReferrerPolicy)
		}
		if cfg.PermissionsPolicy != "" {
			c.Set("Permissions-Policy", cfg.PermissionsPolicy)
		}
		if cfg.CSP != "" {
			c.Set("Content-Security-Policy", cfg.CSP)
		}
		if cfg.HSTS {
			val := hstsValue(cfg.HSTSMaxAge, cfg.HSTSIncludeSubs)
			c.Set("Strict-Transport-Security", val)
		}
		if cfg.COOP != "" {
			c.Set("Cross-Origin-Opener-Policy", cfg.COOP)
		}
		if cfg.COEP != "" {
			c.Set("Cross-Origin-Embedder-Policy", cfg.COEP)
		}
		if cfg.CORP != "" {
			c.Set("Cross-Origin-Resource-Policy", cfg.CORP)
		}
		if cfg.CacheControl != "" {
			c.Set("Cache-Control", cfg.CacheControl)
		}
		return c.Next()
	}
}

func hstsValue(maxAge int, includeSubs bool) string {
	val := "max-age=" + itoa(maxAge)
	if includeSubs {
		val += "; includeSubDomains"
	}
	return val
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := make([]byte, 0, 10)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		digits = append(digits, byte('0'+n%10))
		n /= 10
	}
	if neg {
		digits = append(digits, '-')
	}
	for i, j := 0, len(digits)-1; i < j; i, j = i+1, j-1 {
		digits[i], digits[j] = digits[j], digits[i]
	}
	return string(digits)
}
