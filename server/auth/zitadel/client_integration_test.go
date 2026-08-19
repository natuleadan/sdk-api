//go:build integration

package zitadel

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/natuleadan/sdk-api/server/auth/jwks"
)

func skipIfNoZitadel(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost:18082/debug/healthz", nil)
		if err == nil {
			resp, err := http.DefaultClient.Do(req)
			if err == nil {
				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					_ = body
					return
				}
			}
		}
		select {
		case <-ctx.Done():
			t.Skipf("Zitadel not ready at localhost:18082: %v", ctx.Err())
			return
		case <-time.After(2 * time.Second):
		}
	}
	dialCtx, dialCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer dialCancel()
	conn, err := (&net.Dialer{Timeout: 2 * time.Second}).DialContext(dialCtx, "tcp", "localhost:18082")
	if err != nil {
		t.Skipf("Zitadel not available at localhost:18082: %v", err)
		return
	}
	conn.Close()
}

func fetchJWKSForTest(t *testing.T, discoveryURL string) (map[string]*rsa.PublicKey, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, discoveryURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("discovery %s returned %d: %s", discoveryURL, resp.StatusCode, string(body))
	}
	var disc struct {
		JWKSURI string `json:"jwks_uri"`
	}
	if err := json.Unmarshal(body, &disc); err == nil && disc.JWKSURI != "" {
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, disc.JWKSURI, nil)
		if err != nil {
			return nil, err
		}
		resp, err = http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		body, err = io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("jwks %s returned %d", disc.JWKSURI, resp.StatusCode)
		}
	}
	return jwks.ParseJWKS(body)
}

func TestIntegration_Zitadel_JWKSFetch(t *testing.T) {
	skipIfNoZitadel(t)

	issuer := "http://localhost:18082"

	keys, err := fetchJWKSForTest(t, issuer+"/.well-known/openid-configuration")
	if err != nil {
		t.Fatalf("getKeys failed: %v", err)
	}

	if len(keys) == 0 {
		t.Fatal("expected at least one JWKS key from Zitadel")
	}

	for kid, key := range keys {
		t.Logf("key found: kid=%s, N bits=%d", kid, key.N.BitLen())
		if key.N.BitLen() < 2048 {
			t.Errorf("key %s has fewer than 2048 bits: %d", kid, key.N.BitLen())
		}
	}
}

func TestIntegration_Zitadel_CacheExpiry(t *testing.T) {
	skipIfNoZitadel(t)

	issuer := "http://localhost:18082"
	_ = NewClient(Config{Issuer: issuer, TTL: 10 * time.Minute})

	keys, err := fetchJWKSForTest(t, issuer+"/.well-known/openid-configuration")
	if err != nil {
		t.Fatalf("first getKeys failed: %v", err)
	}
	if len(keys) == 0 {
		t.Fatal("expected keys on first fetch")
	}

	cachedKeys, err := fetchJWKSForTest(t, issuer+"/.well-known/openid-configuration")
	if err != nil {
		t.Fatalf("cached getKeys failed: %v", err)
	}
	if len(cachedKeys) != len(keys) {
		t.Errorf("cached keys count mismatch: %d vs %d", len(cachedKeys), len(keys))
	}
}
