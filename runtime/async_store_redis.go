package runtime

import (
	"context"
	"encoding/json"
	"time"

	"github.com/natuleadan/sdk-api/infra/logx"
	"github.com/natuleadan/sdk-api/infra/stores/redis"
)

// redisJobStore persists job state in Redis.
type redisJobStore struct {
	client            *redis.Redis
	prefix            string
	processingTimeout time.Duration
}

func newRedisJobStore(client *redis.Redis, prefix string) *redisJobStore {
	return &redisJobStore{client: client, prefix: prefix, processingTimeout: 5 * time.Minute}
}

func (s *redisJobStore) key(id string) string {
	return s.prefix + id
}

func (s *redisJobStore) Create(id string) *JobState {
	now := time.Now()
	js := &JobState{ID: id, Status: JobPending, CreatedAt: now, UpdatedAt: now}
	data, _ := json.Marshal(js)
	if err := s.client.SetCtx(context.Background(), s.key(id), string(data)); err != nil {
		logx.Errorf("redisJobStore.Create: %v", err)
	}
	return js
}

func (s *redisJobStore) Get(id string) (*JobState, bool) {
	data, err := s.client.GetCtx(context.Background(), s.key(id))
	if err != nil {
		return nil, false
	}
	var js JobState
	if err := json.Unmarshal([]byte(data), &js); err != nil {
		return nil, false
	}
	return &js, true
}

func (s *redisJobStore) Update(id string, status JobStatus, result any, errMsg string) {
	js, ok := s.Get(id)
	if !ok {
		return
	}
	js.Status = status
	js.Result = result
	js.Error = errMsg
	js.UpdatedAt = time.Now()
	if status == JobProcessing {
		dl := time.Now().Add(s.processingTimeout)
		js.ProcessingDeadline = &dl
	} else {
		js.ProcessingDeadline = nil
	}
	data, _ := json.Marshal(js)
	if err := s.client.SetCtx(context.Background(), s.key(id), string(data)); err != nil {
		logx.Errorf("redisJobStore.Update: %v", err)
	}
}

func (s *redisJobStore) List() ([]*JobState, error) {
	cursor := uint64(0)
	var result []*JobState
	for {
		keys, next, err := s.client.Scan(cursor, s.prefix+"*", 100)
		if err != nil {
			return result, err
		}
		for _, key := range keys {
			data, err := s.client.Get(key)
			if err != nil {
				continue
			}
			var js JobState
			if err := json.Unmarshal([]byte(data), &js); err != nil {
				continue
			}
			result = append(result, &js)
		}
		if next == 0 {
			break
		}
		cursor = next
	}
	return result, nil
}

func (s *redisJobStore) Delete(id string) {
	if _, err := s.client.Del(s.key(id)); err != nil {
		logx.Errorf("redisJobStore.Delete: %v", err)
	}
}

func (s *redisJobStore) ReapStale(_ context.Context, timeout time.Duration, maxRetries int) (int, error) {
	cursor := uint64(0)
	var count int
	for {
		keys, next, err := s.client.Scan(cursor, s.prefix+"*", 100)
		if err != nil {
			return count, err
		}
		now := time.Now()
		for _, key := range keys {
			data, err := s.client.Get(key)
			if err != nil {
				continue
			}
			var js JobState
			if err := json.Unmarshal([]byte(data), &js); err != nil {
				continue
			}
			if js.Status != JobProcessing {
				continue
			}
			if js.ProcessingDeadline == nil || now.Before(*js.ProcessingDeadline) {
				continue
			}
			if js.RetryCount >= maxRetries {
				js.Status = JobFailed
				js.Error = "max retries exceeded"
				js.ProcessingDeadline = nil
				js.UpdatedAt = now
			} else {
				js.Status = JobPending
				js.RetryCount++
				js.ProcessingDeadline = nil
				js.UpdatedAt = now
			}
			updated, _ := json.Marshal(js)
			if err := s.client.SetCtx(context.Background(), key, string(updated)); err != nil {
				logx.Errorf("redisJobStore.ReapStale: %v", err)
				continue
			}
			count++
		}
		if next == 0 {
			break
		}
		cursor = next
	}
	return count, nil
}

func (s *redisJobStore) Cleanup(_ context.Context, ttl time.Duration) (int, error) {
	cutoff := time.Now().Add(-ttl)
	cursor := uint64(0)
	var count int
	for {
		keys, next, err := s.client.Scan(cursor, s.prefix+"*", 100)
		if err != nil {
			return count, err
		}
		for _, key := range keys {
			data, err := s.client.Get(key)
			if err != nil {
				continue
			}
			var js JobState
			if err := json.Unmarshal([]byte(data), &js); err != nil {
				continue
			}
			if (js.Status == JobCompleted || js.Status == JobFailed) && js.UpdatedAt.Before(cutoff) {
				if _, delErr := s.client.Del(key); delErr != nil {
					logx.Errorf("redisJobStore.Cleanup: delete %s: %v", key, delErr)
				}
				count++
			}
		}
		if next == 0 {
			break
		}
		cursor = next
	}
	return count, nil
}
