package middleware

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/natuleadan/sdk-api/infra/logx"
)

func TestBodyReader_CachesBody(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(BodyReader())
	app.Post("/test", func(c fiber.Ctx) error {
		cached := getRequestBody(c)
		direct := c.Body()
		if string(cached) != string(direct) {
			t.Errorf("cached body %q != direct body %q", string(cached), string(direct))
		}
		return c.SendString("ok")
	})

	req, _ := http.NewRequestWithContext(context.Background(), "POST", "/test", strings.NewReader(`{"name":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestBodyReader_FallbackWithoutBodyReader(t *testing.T) {
	app := fiber.New()
	app.Post("/test", func(c fiber.Ctx) error {
		body := getRequestBody(c)
		if len(body) == 0 {
			t.Error("expected non-empty body from fallback")
		}
		return c.SendString("ok")
	})

	req, _ := http.NewRequestWithContext(context.Background(), "POST", "/test", strings.NewReader("fallback"))
	resp, _ := app.Test(req)
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestSetRequestBody_UpdatesBoth(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(BodyReader())
	app.Use(func(c fiber.Ctx) error {
		setRequestBody(c, []byte(`{"transformed":true}`))
		return c.Next()
	})
	app.Post("/test", func(c fiber.Ctx) error {
		cached := getRequestBody(c)
		if string(cached) != `{"transformed":true}` {
			t.Errorf("expected transformed body, got %q", string(cached))
		}
		direct := c.Body()
		if string(direct) != `{"transformed":true}` {
			t.Errorf("expected transformed body from c.Body(), got %q", string(direct))
		}
		return c.SendString("ok")
	})

	req, _ := http.NewRequestWithContext(context.Background(), "POST", "/test", strings.NewReader(`{"original":true}`))
	resp, _ := app.Test(req)
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestMaxBytesWithBodyReader(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(BodyReader())
	app.Use(MaxBytes(10))
	app.Post("/test", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req, _ := http.NewRequestWithContext(context.Background(), "POST", "/test", strings.NewReader("12345678901"))
	resp, _ := app.Test(req)
	if resp.StatusCode != 413 {
		t.Errorf("expected 413 for oversized body, got %d", resp.StatusCode)
	}
}

func TestBufferPool(t *testing.T) {
	buf := getBuffer()
	if buf == nil {
		t.Fatal("expected non-nil buffer")
	}
	buf.WriteString("test")
	if buf.String() != "test" {
		t.Errorf("expected 'test', got %q", buf.String())
	}
	putBuffer(buf)

	buf2 := getBuffer()
	if buf2.String() != "" {
		t.Errorf("expected empty buffer after getBuffer, got %q", buf2.String())
	}
	putBuffer(buf2)
}

func TestBufferPool_LargeBufferDiscarded(t *testing.T) {
	large := getBuffer()
	for range 2 << 20 {
		large.WriteByte('x')
	}
	putBuffer(large)

	small := getBuffer()
	defer putBuffer(small)
	_ = small
}

func TestGunzipWithBodyReader(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Use(BodyReader())
	app.Use(Gunzip())
	app.Post("/test", func(c fiber.Ctx) error {
		body := getRequestBody(c)
		if string(body) != "ok" {
			t.Errorf("expected decompressed 'ok', got %q", string(body))
		}
		return c.SendString("decompressed:" + string(body))
	})

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	gw.Write([]byte("ok"))
	gw.Close()

	req, _ := http.NewRequestWithContext(context.Background(), "POST", "/test", bytes.NewReader(buf.Bytes()))
	resp, _ := app.Test(req)
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "decompressed:ok") {
		t.Errorf("expected decompressed body, got %s", string(body))
	}
}
