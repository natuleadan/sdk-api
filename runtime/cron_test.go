package runtime

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestCronScheduler_AddJob_Handler(t *testing.T) {
	s := NewCronScheduler()
	var called atomic.Int64

	err := s.AddJob(context.Background(), CronJob{
		Name:     "test",
		Schedule: "@every 1s",
		Mode:     "handler",
		Handler:  "onTest",
	}, nil, func(_ context.Context) error {
		called.Add(1)
		return nil
	})
	if err != nil {
		t.Fatalf("AddJob: %v", err)
	}

	s.Start()
	time.Sleep(1500 * time.Millisecond)
	s.Stop()

	if called.Load() < 1 {
		t.Error("handler should have been called at least once")
	}
	t.Logf("called: %d times", called.Load())
}

func TestCronScheduler_AddJob_Duplicate(t *testing.T) {
	s := NewCronScheduler()
	err := s.AddJob(context.Background(), CronJob{
		Name: "dup", Schedule: "@every 1s", Mode: "handler",
	}, nil, func(_ context.Context) error { return nil })
	if err != nil {
		t.Fatalf("first AddJob: %v", err)
	}

	err = s.AddJob(context.Background(), CronJob{
		Name: "dup", Schedule: "@every 1s", Mode: "handler",
	}, nil, func(_ context.Context) error { return nil })
	if err == nil {
		t.Error("expected error for duplicate job")
	}
}

func TestCronScheduler_AddAfterStart(t *testing.T) {
	s := NewCronScheduler()
	s.Start()
	defer s.Stop()

	err := s.AddJob(context.Background(), CronJob{
		Name: "late", Schedule: "@every 1s", Mode: "handler",
	}, nil, func(_ context.Context) error { return nil })
	if err == nil {
		t.Error("expected error adding job after start")
	}
}

func TestCronScheduler_ModeNats_RequiresConnection(t *testing.T) {
	s := NewCronScheduler()
	err := s.AddJob(context.Background(), CronJob{
		Name: "nats-job", Schedule: "@every 1s", Mode: "nats",
		Publish: &CronPublish{Stream: "s", Subject: "s"},
	}, nil, nil) // nil NATS conn
	if err == nil {
		t.Error("expected error for nats mode without connection")
	}
}

func TestCronScheduler_ModeHandler_RequiresFunc(t *testing.T) {
	s := NewCronScheduler()
	err := s.AddJob(context.Background(), CronJob{
		Name: "no-func", Schedule: "@every 1s", Mode: "handler",
	}, nil, nil) // nil handler
	if err == nil {
		t.Error("expected error for handler mode without function")
	}
}

func TestCronScheduler_InternalMode(t *testing.T) {
	s := NewCronScheduler()
	err := s.AddJob(context.Background(), CronJob{
		Name: "internal", Schedule: "@every 1h", Mode: "internal",
	}, nil, nil)
	if err != nil {
		t.Fatalf("AddJob internal: %v", err)
	}
	s.Stop()
}

func TestCronScheduler_UnknownMode(t *testing.T) {
	s := NewCronScheduler()
	err := s.AddJob(context.Background(), CronJob{
		Name: "bad", Schedule: "@every 1s", Mode: "unknown",
	}, nil, nil)
	if err == nil {
		t.Error("expected error for unknown mode")
	}
}

func TestCronScheduler_AddAll(t *testing.T) {
	s := NewCronScheduler()
	var called atomic.Int64

	cronDefs := []CronJob{
		{Name: "every-second", Schedule: "@every 1s", Mode: "handler", Handler: "tick"},
	}

	handlers := map[string]CronJobFunc{
		"tick": func(_ context.Context) error {
			called.Add(1)
			return nil
		},
	}

	err := s.AddAll(context.Background(), cronDefs, nil, handlers)
	if err != nil {
		t.Fatalf("AddAll: %v", err)
	}

	s.Start()
	time.Sleep(1500 * time.Millisecond)
	s.Stop()

	if called.Load() < 1 {
		t.Error("handler should have been called at least once")
	}
}

func TestCronScheduler_StopWait(t *testing.T) {
	s := NewCronScheduler()
	var running atomic.Int64

	err := s.AddJob(context.Background(), CronJob{
		Name:     "wait",
		Schedule: "@every 1s",
		Mode:     "handler",
	}, nil, func(_ context.Context) error {
		running.Add(1)
		time.Sleep(300 * time.Millisecond)
		running.Add(-1)
		return nil
	})
	if err != nil {
		t.Fatalf("AddJob: %v", err)
	}

	s.Start()
	time.Sleep(200 * time.Millisecond) // let it start
	s.Stop()

	if running.Load() != 0 {
		t.Error("job still running after stop")
	}
}

func TestService_WithCron(t *testing.T) {
	svc := &Service{
		config: &ServiceConfig{Name: "test", Port: 19060},
	}

	svc.WithCron("daily-report", func(_ context.Context) error { return nil })

	if svc.cronFuncs["daily-report"] == nil {
		t.Error("cron handler not registered")
	}
}
