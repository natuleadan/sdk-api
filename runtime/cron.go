package runtime

import (
	"context"
	"fmt"
	"sync"

	"github.com/natuleadan/sdk-api/events"
	"github.com/natuleadan/sdk-api/infra/logx"
	"github.com/robfig/cron/v3"
)

type CronJobFunc func(ctx context.Context) error

type CronScheduler struct {
	cron    *cron.Cron
	jobs    map[string]cron.EntryID
	mu      sync.Mutex
	started bool
}

func NewCronScheduler() *CronScheduler {
	return &CronScheduler{
		cron: cron.New(),
		jobs: make(map[string]cron.EntryID),
	}
}

func (s *CronScheduler) AddJob(ctx context.Context, cfg CronJob, broker events.EventBroker, handler CronJobFunc) error {
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
		if broker == nil {
			return fmt.Errorf("cron %q mode=nats requires event broker", cfg.Name)
		}
		subject := cfg.Publish.Subject
		if subject == "" {
			subject = cfg.Publish.Stream
		}
		b := broker
		entryID, err = s.cron.AddFunc(cfg.Schedule, func() {
			if pubErr := b.Publish(context.Background(), subject, nil); pubErr != nil {
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

func (s *CronScheduler) AddAll(ctx context.Context, cronDefs []CronJob, brokers map[string]events.EventBroker, handlers map[string]CronJobFunc) error {
	var defaultBroker events.EventBroker
	for _, b := range brokers {
		defaultBroker = b
		break
	}

	for _, cfg := range cronDefs {
		var handler CronJobFunc
		if cfg.Mode == "handler" {
			if handlers != nil {
				handler = handlers[cfg.Handler]
			}
		}
		if err := s.AddJob(ctx, cfg, defaultBroker, handler); err != nil {
			return err
		}
	}
	return nil
}

func (s *CronScheduler) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.started {
		s.cron.Start()
		s.started = true
		logx.Info("cron scheduler started")
	}
}

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
