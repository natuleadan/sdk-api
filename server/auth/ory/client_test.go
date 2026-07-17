package ory

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"math/big"
	"testing"
)

func TestParseJWKS_Valid(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pub := &priv.PublicKey

	n := base64.RawURLEncoding.EncodeToString(pub.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes())

	body := []byte(`{"keys":[{"kid":"test-key-1","kty":"RSA","n":"` + n + `","e":"` + e + `"}]}`)

	keys, err := parseJWKS(body)
	if err != nil {
		t.Fatalf("parseJWKS failed: %v", err)
	}

	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}

	parsedKey, ok := keys["test-key-1"]
	if !ok {
		t.Fatal("key test-key-1 not found")
	}

	if parsedKey.N.Cmp(pub.N) != 0 {
		t.Error("parsed key N does not match")
	}
}

func TestParseJWKS_NonRSA(t *testing.T) {
	body := []byte(`{"keys":[{"kid":"ec-key","kty":"EC","crv":"P-256","x":"...","y":"..."}]}`)

	keys, err := parseJWKS(body)
	if err != nil {
		t.Fatalf("parseJWKS failed: %v", err)
	}

	if len(keys) != 0 {
		t.Errorf("expected 0 keys for non-RSA, got %d", len(keys))
	}
}

func TestParseJWKS_InvalidBase64(t *testing.T) {
	body := []byte(`{"keys":[{"kid":"bad-key","kty":"RSA","n":"!!!invalid!!!","e":"AQAB"}]}`)

	keys, err := parseJWKS(body)
	if err != nil {
		t.Fatalf("parseJWKS failed: %v", err)
	}

	if len(keys) != 0 {
		t.Errorf("expected 0 keys for invalid base64, got %d", len(keys))
	}
}

func TestParseJWKS_InvalidJSON(t *testing.T) {
	body := []byte(`not json`)

	_, err := parseJWKS(body)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestParseJWKS_Empty(t *testing.T) {
	body := []byte(`{"keys":[]}`)

	keys, err := parseJWKS(body)
	if err != nil {
		t.Fatalf("parseJWKS failed: %v", err)
	}

	if len(keys) != 0 {
		t.Errorf("expected 0 keys for empty set, got %d", len(keys))
	}
}
