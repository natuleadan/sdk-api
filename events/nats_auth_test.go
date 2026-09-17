package events

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nkeys"
)

// TestConnectAuthOptions_Params verifies the new auth options fail fast on
// an unreachable endpoint instead of hanging or panicking. Live servers are
// covered by nats_auth_integration_test.go (build tag integration).
func TestConnectAuthOptions_Params(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := Connect(ctx, ConnOptions{
		URL:          "nats://127.0.0.1:1", // nothing listens on :1
		Timeout:      2 * time.Second,
		Token:        "s3cr3t",
		NKeySeed:     "SUINVALIDSEED",
		NKeySeedFile: "/nonexistent/nkey.seed",
		CredsFile:    "/nonexistent/user.creds",
	})
	if err == nil {
		if conn != nil {
			conn.NC.Close()
		}
		t.Fatal("expected connect error for unreachable endpoint")
	}
}

func TestNkeyOption_InlineSeed(t *testing.T) {
	kp, err := nkeys.CreateUser()
	if err != nil {
		t.Fatal(err)
	}
	seed, err := kp.Seed()
	if err != nil {
		t.Fatal(err)
	}
	opt, err := nkeyOption(ConnOptions{NKeySeed: string(seed)})
	if err != nil {
		t.Fatalf("valid inline seed: %v", err)
	}
	if opt == nil {
		t.Fatal("expected non-nil option for valid seed")
	}
	if _, err := nkeyOption(ConnOptions{NKeySeed: "not-a-seed"}); err == nil {
		t.Error("expected error for invalid seed, got nil")
	}
}

func TestNkeyOption_SeedFile(t *testing.T) {
	kp, err := nkeys.CreateUser()
	if err != nil {
		t.Fatal(err)
	}
	seed, err := kp.Seed()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "user.nk")
	if err := os.WriteFile(path, append(seed, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	opt, err := nkeyOption(ConnOptions{NKeySeedFile: path})
	if err != nil {
		t.Fatalf("valid seed file: %v", err)
	}
	if opt == nil {
		t.Fatal("expected non-nil option for valid seed file")
	}
	if _, err := nkeyOption(ConnOptions{NKeySeedFile: "/nonexistent/nkey.seed"}); err == nil {
		t.Error("expected error for missing seed file, got nil")
	}
	if opt, err := nkeyOption(ConnOptions{}); err != nil || opt != nil {
		t.Errorf("empty opts = (%v, %v), want (nil, nil)", opt, err)
	}
}

// mintTestCreds builds a Synadia-shaped .creds file (user JWT + NKey seed)
// signed through a fresh operator→account→user chain. The server side is
// covered live (integration); here it proves client-side parsing.
func mintTestCreds(t *testing.T) (credsPath string) {
	t.Helper()
	accKP, err := nkeys.CreateAccount()
	if err != nil {
		t.Fatal(err)
	}
	userKP, err := nkeys.CreateUser()
	if err != nil {
		t.Fatal(err)
	}
	userPub, _ := userKP.PublicKey()
	userSeed, _ := userKP.Seed()

	issue := func(issuer nkeys.KeyPair, sub, name string) string {
		t.Helper()
		issuerPub, err := issuer.PublicKey()
		if err != nil {
			t.Fatal(err)
		}
		header, _ := json.Marshal(map[string]string{"typ": "JWT", "alg": "ed25519-nkey"})
		payload, _ := json.Marshal(map[string]any{
			"jti":  fmt.Sprintf("%d", time.Now().UnixNano()),
			"iat":  time.Now().Unix(),
			"iss":  issuerPub,
			"sub":  sub,
			"name": name,
			"nats": map[string]any{},
		})
		enc := base64.RawURLEncoding.EncodeToString
		unsigned := enc(header) + "." + enc(payload)
		sig, err := issuer.Sign([]byte(unsigned))
		if err != nil {
			t.Fatal(err)
		}
		return unsigned + "." + enc(sig)
	}
	userJWT := issue(accKP, userPub, "test-user")

	creds := "-----BEGIN NATS USER JWT-----\n" + userJWT + "\n------END NATS USER JWT------\n\n" +
		"-----BEGIN USER NKEY SEED-----\n" + string(userSeed) + "\n------END USER NKEY SEED------\n"
	credsPath = filepath.Join(t.TempDir(), "test.creds")
	if err := os.WriteFile(credsPath, []byte(creds), 0o600); err != nil {
		t.Fatal(err)
	}
	return credsPath
}

// TestCredsFile_ParsesBeforeDial proves a well-formed creds file gets past
// client parsing: against an unreachable server the failure must be the
// connection (not a credentials error).
func TestCredsFile_ParsesBeforeDial(t *testing.T) {
	credsPath := mintTestCreds(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := Connect(ctx, ConnOptions{
		URL:       "nats://127.0.0.1:1",
		Timeout:   2 * time.Second,
		CredsFile: credsPath,
	})
	if err == nil {
		if conn != nil {
			conn.NC.Close()
		}
		t.Fatal("expected connect error for unreachable endpoint")
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "credential") || strings.Contains(msg, "creds") ||
		strings.Contains(msg, "jwt") || strings.Contains(msg, "nkey") {
		t.Fatalf("creds should parse cleanly, got parse error: %v", err)
	}
	if !strings.Contains(msg, "no servers available") {
		t.Fatalf("expected dial failure, got: %v", err)
	}
}
