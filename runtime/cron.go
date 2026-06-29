package runtime

import (
	"context"
	"fmt"
	"sync"

	"github.com/natuleadan/sdk-api/events"
	"github.com/natuleadan/sdk-api/infra/logx"
	"github.com/robfig/cron/v3"
)

// CronJobFunc is a handler called on cron schedule. Returns error on failure.
type CronJobFunc func(ctx context.Context) error

// CronScheduler manages periodic jobs defined in cron:[].
type CronScheduler struct {
	cron      *cron.Cron
	jobs      map[string]cron.EntryID
	natsConns map[string]*events.Conn
	mu        sync.Mutex
	started   bool
}

// NewCronScheduler creates a new scheduler.
func NewCronScheduler() *CronScheduler {
	return &CronScheduler{
		cron:  cron.New(),
		jobs:  make(map[string]cron.EntryID),
	}
}

// AddJob registers a cron job from config. handler is only used for mode=handler.
// nats is used for mode=nats to publish on schedule.
func (s *CronScheduler) AddJob(cfg CronJob, natsConn *events.Conn, handler CronJobFunc) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return fmt.Errorf("cron: cannot add job %q after Start()", cfg.Name)
	}
	if _, exists := s.jobs[cfg.Name]; exists {
		return fmt.Errorf("cron: duplicate job %q", cfg.Name)
	}

	var entryID cron.EntryID
	var err error

	switch cfg.Mode {
	case "nats":
		if natsConn == nil {
			return fmt.Errorf("cron %q mode=nats requires NATS connection", cfg.Name)
		}
		stream := cfg.Publish.Stream
		subject := cfg.Publish.Subject
		if subject == "" {
			subject = stream
		}
		js := natsConn.JS
		entryID, err = s.cron.AddFunc(cfg.Schedule, func() {
			_, pubErr := js.Publish(subject, nil)
			if pubErr != nil {
				logx.Errorf("cron %s publish %s: %v", cfg.Name, subject, pubErr)
			} else {
				logx.Infof("cron %s published to %s", cfg.Name, subject)
			}
		})

	case "handler":
		if handler == nil {
			return fmt.Errorf("cron %q mode=handler requires a handler function", cfg.Name)
		}
		h := handler
		entryID, err = s.cron.AddFunc(cfg.Schedule, func() {
			if hErr := h(context.Background()); hErr != nil {
				logx.Errorf("cron %s handler error: %v", cfg.Name, hErr)
			}
		})

	case "internal":
		entryID, err = s.cron.AddFunc(cfg.Schedule, func() {
			logx.Infof("cron %s (internal) tick", cfg.Name)
		})

	default:
		return fmt.Errorf("unknown cron mode %q", cfg.Mode)
	}

	if err != nil {
		return fmt.Errorf("cron %q: %w", cfg.Name, err)
	}

	s.jobs[cfg.Name] = entryID
	logx.Infof("cron registered: %s schedule=%s mode=%s", cfg.Name, cfg.Schedule, cfg.Mode)
	return nil
}

// AddAll registers all cron jobs from config.
func (s *CronScheduler) AddAll(cronDefs []CronJob, natsConns map[string]*events.Conn, handlers map[string]CronJobFunc) error {
	// Select the first NATS connection as default for nats-mode jobs
	var defaultNats *events.Conn
	for _, conn := range natsConns {
		defaultNats = conn
		break
	}

	for _, cfg := range cronDefs {
		var handler CronJobFunc
		if cfg.Mode == "handler" {
			if handlers != nil {
				handler = handlers[cfg.Handler]
			}
		}
		if err := s.AddJob(cfg, defaultNats, handler); err != nil {
			return err
		}
	}
	return nil
}

// Start begins executing scheduled jobs.
func (s *CronScheduler) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.started {
		s.cron.Start()
		s.started = true
		logx.Info("cron scheduler started")
	}
}

// Stop stops the scheduler and waits for running jobs to finish.
func (s *CronScheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		ctx := s.cron.Stop()
		<-ctx.Done()
		s.started = false
		logx.Info("cron scheduler stopped")
	}
}
