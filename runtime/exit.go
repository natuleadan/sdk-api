package runtime

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/natuleadan/sdk-api/events"
	"github.com/natuleadan/sdk-api/infra/logx"
)

type ExitHandler func(ctx context.Context, msg []byte) ([]byte, error)

type workerState struct {
	shutdownCh chan struct{}
	tasks      atomic.Int64
}

type exitWorker struct {
	name    string
	cfg     ExitWorker
	handler ExitHandler
	hooks   ExitHooks
	sub     events.Subscription
	sem     chan struct{}
	state   *workerState
}

func startExitWorker(ctx context.Context, broker events.EventBroker, cfg ExitWorker, handler ExitHandler, hooks ExitHooks) (*exitWorker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if handler == nil {
		return nil, fmt.Errorf("exit %q: handler is nil", cfg.Name)
	}

	state := &workerState{
		shutdownCh: make(chan struct{}, 1),
	}

	w := &exitWorker{
		name:    cfg.Name,
		cfg:     cfg,
		handler: handler,
		hooks:   hooks,
		sem:     make(chan struct{}, cfg.MaxConcurrent),
		state:   state,
	}

	if cfg.PullBatch > 0 || strings.ToLower(cfg.ConsumerMode) == "pull" {
		consumer, err := broker.PullSubscribe(ctx, cfg.Subscribe.Subject, cfg.Subscribe.Durable)
		if err != nil {
			return nil, fmt.Errorf("exit %q pull subscribe: %w", cfg.Name, err)
		}
		w.sub = consumer

		pullBatch := cfg.PullBatch
		if pullBatch <= 0 {
			pullBatch = 10
		}
		pullMaxWait := parseServerDuration(cfg.PullMaxWait, 5*time.Second)

		sem := w.sem
		handler := w.handler
		hooks := w.hooks
		cfgW := w.cfg
		nameW := w.name
		stateW := w.state
		shutdownCh := state.shutdownCh
		go func() {
			defer func() { _ = consumer.Unsubscribe() }()
			for {
				select {
				case <-shutdownCh:
					return
				default:
				}
				msgs, fetchErr := consumer.Fetch(pullBatch, pullMaxWait)
				if fetchErr != nil {
					continue
				}
				for _, m := range msgs {
					processMsg(stateW, sem, handler, hooks, cfgW, nameW, m)
				}
			}
		}()
	} else {
		sub, err := broker.Subscribe(ctx, cfg.Subscribe.Subject, cfg.Subscribe.Durable, func(ctx context.Context, msg events.Message) error {
			processMsg(state, w.sem, w.handler, w.hooks, w.cfg, w.name, msg)
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("exit %q subscribe: %w", cfg.Name, err)
		}
		w.sub = sub

		shutdownCh := state.shutdownCh
		go func() {
			<-shutdownCh
			_ = sub.Unsubscribe()
		}()
	}

	logx.Infof("exit worker started: %s stream=%s subject=%s concurrent=%d reply=%v",
		cfg.Name, cfg.Subscribe.Stream, cfg.Subscribe.Subject, cfg.MaxConcurrent, cfg.Reply)

	return w, nil
}

func nakWithLog(m events.Message, name, context string) {
	if err := m.Nak(); err != nil {
		logx.Errorf("exit %s nak error (%s): %v", name, context, err)
	}
}

func processMsg(state *workerState, sem chan struct{}, handler ExitHandler, hooks ExitHooks, cfg ExitWorker, name string, m events.Message) {
	select {
	case sem <- struct{}{}:
	case <-state.shutdownCh:
		nakWithLog(m, name, "shutdown")
		return
	}

	state.tasks.Add(1)
	go func() {
		defer func() {
			<-sem
			state.tasks.Add(-1)
		}()

		msg := m.Data()
		if hooks != nil {
			var err error
			msg, err = hooks.OnMessage(context.Background(), m.Data())
			if err != nil {
				logx.Errorf("exit %s onMessage hook: %v", name, err)
				nakWithLog(m, name, "hook")
				return
			}
		}

		resp, err := handler(context.Background(), msg)
		if err != nil {
			if hooks != nil {
				hooks.OnError(context.Background(), err)
			}
			logx.Errorf("exit %s handler error: %v", name, err)
			nakWithLog(m, name, "handler")
			return
		}

		if hooks != nil {
			hooks.OnSuccess(context.Background())
		}

		if cfg.Reply && len(resp) > 0 {
			if rErr := m.Respond(resp); rErr != nil {
				logx.Errorf("exit %s reply error: %v", name, rErr)
				nakWithLog(m, name, "reply")
				return
			}
		}

		if err := m.Ack(); err != nil {
			logx.Errorf("exit %s ack error: %v", name, err)
		}
	}()
}

func (w *exitWorker) shutdown(timeout time.Duration) {
	logx.Infof("exit worker %s shutting down...", w.name)
	w.state.shutdownCh <- struct{}{}

	done := make(chan struct{})
	go func() {
		tick := time.NewTicker(10 * time.Millisecond)
		defer tick.Stop()
		for {
			if w.state.tasks.Load() == 0 {
				close(done)
				return
			}
			select {
			case <-tick.C:
			case <-time.After(timeout):
				close(done)
				return
			}
		}
	}()

	select {
	case <-done:
		logx.Infof("exit worker %s drained", w.name)
	case <-time.After(timeout):
		logx.Errorf("exit worker %s shutdown timeout after %v", w.name, timeout)
	}
}

type ExitWorkerManager struct {
	workers []*exitWorker
}

func NewExitWorkerManager() *ExitWorkerManager {
	return &ExitWorkerManager{}
}

func (m *ExitWorkerManager) Start(ctx context.Context, exitDefs []ExitWorker, brokers map[string]events.EventBroker, handlers map[string]ExitHandler, hooks map[string]ExitHooks) error {
	for _, cfg := range exitDefs {
		var broker events.EventBroker
		for _, b := range brokers {
			broker = b
			break
		}
		if broker == nil {
			return fmt.Errorf("exit %q: no event broker available", cfg.Name)
		}

		handler, ok := handlers[cfg.Handler]
		if !ok {
			return fmt.Errorf("exit %q: handler %q not registered", cfg.Name, cfg.Handler)
		}

		var eh ExitHooks
		if hooks != nil {
			eh = hooks[cfg.Name]
		}

		w, err := startExitWorker(ctx, broker, cfg, handler, eh)
		if err != nil {
			return fmt.Errorf("exit %q: %w", cfg.Name, err)
		}
		m.workers = append(m.workers, w)
	}
	return nil
}

func (m *ExitWorkerManager) Shutdown(timeout time.Duration) {
	for _, w := range m.workers {
		w.shutdown(timeout)
	}
	m.workers = nil
}
