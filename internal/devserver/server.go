package devserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/pprof"
	"sync"
	"time"

	"github.com/natuleadan/sdk-api/infra/logx"
	"github.com/natuleadan/sdk-api/infra/prometheus"
	"github.com/natuleadan/sdk-api/infra/threading"
	"github.com/natuleadan/sdk-api/internal/health"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var once sync.Once

// Server is an inner http server, expose some useful observability information of app.
// For example, health check, metrics and pprof.
type Server struct {
	config Config
	server *http.ServeMux
	routes []string
}

// NewServer returns a new inner http Server.
func NewServer(config Config) *Server {
	return &Server{
		config: config,
		server: http.NewServeMux(),
	}
}

func (s *Server) addRoutes(c Config) {
	// route path, routes list
	s.handleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		if err := json.NewEncoder(w).Encode(s.routes); err != nil {
			logx.Errorf("devserver: encode error: %v", err)
		}
	})

	// health
	s.handleFunc(s.config.HealthPath, health.CreateHttpHandler(c.HealthResponse))

	// metrics
	if s.config.EnableMetrics {
		// enable prometheus global switch
		prometheus.Enable()
		s.handleFunc(s.config.MetricsPath, promhttp.Handler().ServeHTTP)
	}

	// pprof
	if s.config.EnablePprof {
		s.handleFunc("/debug/pprof/", pprof.Index)
		s.handleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		s.handleFunc("/debug/pprof/profile", pprof.Profile)
		s.handleFunc("/debug/pprof/symbol", pprof.Symbol)
		s.handleFunc("/debug/pprof/trace", pprof.Trace)
	}
}

func (s *Server) handleFunc(pattern string, handler http.HandlerFunc) {
	s.server.HandleFunc(pattern, handler)
	s.routes = append(s.routes, pattern)
}

// StartAsync start inner http server background.
func (s *Server) StartAsync(c Config) {
	s.addRoutes(c)
	threading.GoSafe(func() {
		addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)
		logx.Infof("Starting dev http server at %s", addr)
		srv := &http.Server{
			Addr:         addr,
			Handler:      s.server,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 10 * time.Second,
		}
		if err := srv.ListenAndServe(); err != nil {
			logx.Error(err)
		}
	})
}

// StartAgent start inner http server by config.
func StartAgent(c Config) {
	if !c.Enabled {
		return
	}

	once.Do(func() {
		s := NewServer(c)
		s.StartAsync(c)
	})
}
