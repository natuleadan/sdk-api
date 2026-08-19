package runtime

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/natuleadan/sdk-api/events"
	"github.com/natuleadan/sdk-api/infra/logx"
)

type ExitHandler func(ctx context.Context, msg []byte) ([]byte, error)

type msgTask struct {
	msg events.Message
	cfg ExitWorker
}

type exitWorker struct {
	name     string
	handler  ExitHandler
	hooks    ExitHooks
	sub      events.Subscription
	consumer events.PullConsumer
	tasks    chan msgTask
	state    *workerState
}

type workerState struct {
	shutdownCh chan struct{}
	inFlight   atomic.Int64
	once       sync.Once
}

func startExitWorker(ctx context.Context, broker events.EventBroker, cfg ExitWorker, handler ExitHandler, hooks ExitHooks) (*exitWorker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if handler == nil {
		return nil, fmt.Errorf("exit %q: handler is nil", cfg.Name)
	}

	maxConcurrent := cfg.MaxConcurrent
	if maxConcurrent <= 0 {
		maxConcurrent = 1
	}

	taskCh := make(chan msgTask, maxConcurrent*2)
	state := &workerState{
		shutdownCh: make(chan struct{}),
	}

	w := &exitWorker{
		name:    cfg.Name,
		handler: handler,
		hooks:   hooks,
		tasks:   taskCh,
		state:   state,
	}

	for i := 0; i < maxConcurrent; i++ {
		go workerLoop(ctx, state, taskCh, handler, hooks)
	}

	if cfg.PullBatch > 0 || strings.ToLower(cfg.ConsumerMode) == "pull" {
		consumer, err := broker.PullSubscribe(ctx, cfg.Subscribe.Subject, cfg.Subscribe.Durable)
		if err != nil {
			return nil, fmt.Errorf("exit %q pull subscribe: %w", cfg.Name, err)
		}
		w.consumer = consumer

		pullBatch := cfg.PullBatch
		if pullBatch <= 0 {
			pullBatch = 10
		}
		pullMaxWait := parseServerDuration(cfg.PullMaxWait, 5*time.Second)

		go func() {
			defer func() {
				if err := consumer.Unsubscribe(); err != nil {
					logx.Errorf("exit: consumer unsubscribe error: %v", err)
				}
			}()
			for {
				if err := fetchAndEnqueue(state, taskCh, consumer, pullBatch, pullMaxWait, cfg); err != nil {
					return
				}
			}
		}()
	} else {
		sub, err := broker.Subscribe(ctx, cfg.Subscribe.Subject, cfg.Subscribe.Durable, func(ctx context.Context, msg events.Message) error {
			enqueueMsg(state, taskCh, msgTask{msg: msg, cfg: cfg})
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("exit %q subscribe: %w", cfg.Name, err)
		}
		w.sub = sub
	}

	logx.Infof("exit worker started: %s stream=%s subject=%s concurrent=%d reply=%v",
		cfg.Name, cfg.Subscribe.Stream, cfg.Subscribe.Subject, cfg.MaxConcurrent, cfg.Reply)

	return w, nil
}

func fetchAndEnqueue(state *workerState, taskCh chan<- msgTask, consumer events.PullConsumer, batch int, maxWait time.Duration, cfg ExitWorker) error {
	select {
	case <-state.shutdownCh:
		return fmt.Errorf("shutting down")
	default:
	}

	msgs, err := consumer.Fetch(batch, maxWait)
	if err != nil {
		return nil
	}
	for _, m := range msgs {
		enqueueMsg(state, taskCh, msgTask{msg: m, cfg: cfg})
	}
	return nil
}

func enqueueMsg(state *workerState, taskCh chan<- msgTask, task msgTask) {
	select {
	case taskCh <- task:
	case <-state.shutdownCh:
		nakWithLog(task.msg, task.cfg.Name, "shutdown")
	}
}

func workerLoop(ctx context.Context, state *workerState, taskCh <-chan msgTask, handler ExitHandler, hooks ExitHooks) {
	for task := range taskCh {
		state.inFlight.Add(1)
		processTask(ctx, task, handler, hooks)
		state.inFlight.Add(-1)
	}
}

func processTask(ctx context.Context, task msgTask, handler ExitHandler, hooks ExitHooks) {
	m := task.msg
	cfg := task.cfg
	name := cfg.Name

	msg := m.Data()
	if hooks != nil {
		var err error
		msg, err = hooks.OnMessage(ctx, m.Data())
		if err != nil {
			logx.Errorf("exit %s onMessage hook: %v", name, err)
			nakWithLog(m, name, "hook")
			return
		}
	}

	resp, err := handler(ctx, msg)
	if err != nil {
		if hooks != nil {
			hooks.OnError(ctx, err)
		}
		logx.Errorf("exit %s handler error: %v", name, err)
		if cfg.TermOnFailure {
			termWithLog(m, name, "handler-dlq")
		} else {
			nakWithLog(m, name, "handler")
		}
		return
	}

	if hooks != nil {
		hooks.OnSuccess(ctx)
	}

	if cfg.Reply {
		if len(resp) == 0 {
			logx.Infof("exit %s reply skipped: empty response", name)
		} else if rErr := m.Respond(resp); rErr != nil {
			logx.Errorf("exit %s reply error: %v", name, rErr)
			nakWithLog(m, name, "reply")
			return
		}
	}

	if err := m.Ack(); err != nil {
		logx.Errorf("exit %s ack error: %v", name, err)
	}
}

func nakWithLog(m events.Message, name, context string) {
	if err := m.Nak(); err != nil {
		logx.Errorf("exit %s nak error (%s): %v", name, context, err)
	}
}

func termWithLog(m events.Message, name, context string) {
	if err := m.Term(); err != nil {
		logx.Errorf("exit %s term error (%s): %v", name, context, err)
	}
}

func (w *exitWorker) shutdown(timeout time.Duration) {
	logx.Infof("exit worker %s shutting down...", w.name)
	w.state.once.Do(func() {
		close(w.state.shutdownCh)
		close(w.tasks)
	})

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()

	for {
		if w.state.inFlight.Load() == 0 {
			logx.Infof("exit worker %s drained", w.name)
			break
		}
		select {
		case <-tick.C:
		case <-deadline.C:
			logx.Errorf("exit worker %s shutdown timeout after %v (%d tasks remaining)",
				w.name, timeout, w.state.inFlight.Load())
			return
		}
	}

	if w.sub != nil {
		if err := w.sub.Unsubscribe(); err != nil {
			logx.Errorf("exit: unsubscribe %s error: %v", w.name, err)
		}
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
		if cfg.EventStream != "" {
			var ok bool
			broker, ok = brokers[cfg.EventStream]
			if !ok {
				return fmt.Errorf("exit %q: event_stream %q not found", cfg.Name, cfg.EventStream)
			}
		} else {
			for _, b := range brokers {
				broker = b
				break
			}
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
