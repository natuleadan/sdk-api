package runtime

import (
	"context"
	"encoding/json"
	"time"

	"github.com/natuleadan/sdk-api/events"
	"github.com/natuleadan/sdk-api/infra/logx"
)

// natsKVJobStore persists job state in a NATS KV bucket.
type natsKVJobStore struct {
	conn              *events.Conn
	bucket            string
	processingTimeout time.Duration
}

func newNATSKVJobStore(conn *events.Conn, bucket string) *natsKVJobStore {
	return &natsKVJobStore{conn: conn, bucket: bucket, processingTimeout: 5 * time.Minute}
}

func (s *natsKVJobStore) Create(id string) *JobState {
	now := time.Now()
	js := &JobState{ID: id, Status: JobPending, CreatedAt: now, UpdatedAt: now}
	data, _ := json.Marshal(js)
	if _, err := s.conn.KVPut(s.bucket, id, data); err != nil {
		logx.Errorf("natsKVJobStore.Create: %v", err)
	}
	return js
}

func (s *natsKVJobStore) Get(id string) (*JobState, bool) {
	data, err := s.conn.KVGet(s.bucket, id)
	if err != nil {
		return nil, false
	}
	var js JobState
	if err := json.Unmarshal(data, &js); err != nil {
		return nil, false
	}
	return &js, true
}

func (s *natsKVJobStore) Update(id string, status JobStatus, result any, errMsg string) {
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
	if _, err := s.conn.KVPut(s.bucket, id, data); err != nil {
		logx.Errorf("natsKVJobStore.Update: %v", err)
	}
}

func (s *natsKVJobStore) List() ([]*JobState, error) {
	keys, err := s.conn.KVKeys(s.bucket)
	if err != nil {
		return nil, err
	}
	var result []*JobState
	for _, key := range keys {
		data, err := s.conn.KVGet(s.bucket, key)
		if err != nil {
			continue
		}
		var js JobState
		if err := json.Unmarshal(data, &js); err != nil {
			continue
		}
		result = append(result, &js)
	}
	return result, nil
}

func (s *natsKVJobStore) Delete(id string) {
	if err := s.conn.KVDelete(s.bucket, id); err != nil {
		logx.Errorf("natsKVJobStore.Delete: %v", err)
	}
}

func (s *natsKVJobStore) ReapStale(_ context.Context, timeout time.Duration, maxRetries int) (int, error) {
	keys, err := s.conn.KVKeys(s.bucket)
	if err != nil {
		return 0, err
	}
	now := time.Now()
	var count int
	for _, key := range keys {
		data, err := s.conn.KVGet(s.bucket, key)
		if err != nil {
			continue
		}
		var js JobState
		if err := json.Unmarshal(data, &js); err != nil {
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
		if _, err := s.conn.KVPut(s.bucket, key, updated); err != nil {
			logx.Errorf("natsKVJobStore.ReapStale: %v", err)
			continue
		}
		count++
	}
	return count, nil
}

func (s *natsKVJobStore) Cleanup(_ context.Context, ttl time.Duration) (int, error) {
	keys, err := s.conn.KVKeys(s.bucket)
	if err != nil {
		return 0, err
	}
	cutoff := time.Now().Add(-ttl)
	var count int
	for _, key := range keys {
		data, err := s.conn.KVGet(s.bucket, key)
		if err != nil {
			continue
		}
		var js JobState
		if err := json.Unmarshal(data, &js); err != nil {
			continue
		}
		if (js.Status == JobCompleted || js.Status == JobFailed) && js.UpdatedAt.Before(cutoff) {
			if delErr := s.conn.KVDelete(s.bucket, key); delErr != nil {
				logx.Errorf("natsKVJobStore.Cleanup: delete %s: %v", key, delErr)
			}
			count++
		}
	}
	return count, nil
}
