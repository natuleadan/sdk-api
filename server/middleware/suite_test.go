package middleware

import (
	"context"
	"net/http"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/natuleadan/sdk-api/infra/logx"
	"github.com/stretchr/testify/suite"
)

type MiddlewareSuite struct {
	suite.Suite
	app *fiber.App
}

func (s *MiddlewareSuite) SetupTest() {
	logx.Disable()
	s.app = fiber.New()
}

func (s *MiddlewareSuite) TearDownTest() {
	s.app = nil
}

func (s *MiddlewareSuite) TestTraceSkipPath() {
	s.app.Use(Trace(TraceConfig{
		SkipPaths: []string{"/health"},
	}))
	s.app.Get("/health", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := testRequest(context.Background(), "GET", "/health", nil)
	resp, _ := s.app.Test(req)
	s.Equal(http.StatusOK, resp.StatusCode)
}

func (s *MiddlewareSuite) TestTraceResponseHeader() {
	s.app.Use(Trace(TraceConfig{
		TraceResponseHeader: "X-Trace-Id",
	}))
	s.app.Get("/test", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := testRequest(context.Background(), "GET", "/test", nil)
	resp, _ := s.app.Test(req)
	s.Equal(http.StatusOK, resp.StatusCode)
	s.NotEmpty(resp.Header.Get("X-Trace-Id"))
}

func TestMiddlewareSuite(t *testing.T) {
	suite.Run(t, new(MiddlewareSuite))
}
