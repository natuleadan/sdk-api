//go:build integration

package events

import (
	"context"
	"os"
	"testing"
	"time"
)

// Live NATS authentication matrix. Each case needs a real server; all are
// skipped unless their env vars are set:
//
//	Token: NATS_TOKEN_URL + NATS_TOKEN (server: authorization { token: ... })
//	NKey:  NATS_NKEY_URL + NATS_NKEY_SEED (server: static nkey user)
//	Creds: NATS_CREDS_URL + NATS_CREDS_FILE (Synadia Cloud or operator JWT
//	setup; the .creds file holds the user JWT + NKey seed)
//
// The unit tests in nats_auth_test.go cover option plumbing without servers.
func connectAuth(t *testing.T, opts ConnOptions) *Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := Connect(ctx, opts)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { conn.NC.Close() })
	return conn
}

func pubSubRoundtrip(t *testing.T, conn *Conn, subject string) {
	t.Helper()
	sub, err := conn.NC.SubscribeSync(subject)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer sub.Unsubscribe()
	if err := conn.NC.Publish(subject, []byte("ping")); err != nil {
		t.Fatalf("publish: %v", err)
	}
	msg, err := sub.NextMsg(5 * time.Second)
	if err != nil {
		t.Fatalf("receive: %v", err)
	}
	if string(msg.Data) != "ping" {
		t.Fatalf("data = %q, want ping", msg.Data)
	}
}

func getenvOrSkip(t *testing.T, vars ...string) []string {
	t.Helper()
	out := make([]string, 0, len(vars))
	for _, v := range vars {
		val := os.Getenv(v)
		if val == "" {
			t.Skipf("%s not set, skipping live auth test", v)
		}
		out = append(out, val)
	}
	return out
}

func TestIntegration_NATS_TokenAuth(t *testing.T) {
	vals := getenvOrSkip(t, "NATS_TOKEN_URL", "NATS_TOKEN")
	conn := connectAuth(t, ConnOptions{URL: vals[0], Token: vals[1], Timeout: 5 * time.Second})
	pubSubRoundtrip(t, conn, "auth.token.ping")
}

func TestIntegration_NATS_NKeyAuth(t *testing.T) {
	vals := getenvOrSkip(t, "NATS_NKEY_URL", "NATS_NKEY_SEED")
	conn := connectAuth(t, ConnOptions{URL: vals[0], NKeySeed: vals[1], Timeout: 5 * time.Second})
	pubSubRoundtrip(t, conn, "auth.nkey.ping")
}

func TestIntegration_NATS_CredsFile(t *testing.T) {
	vals := getenvOrSkip(t, "NATS_CREDS_URL", "NATS_CREDS_FILE")
	conn := connectAuth(t, ConnOptions{URL: vals[0], CredsFile: vals[1], Timeout: 10 * time.Second})
	pubSubRoundtrip(t, conn, "auth.creds.ping")
}
