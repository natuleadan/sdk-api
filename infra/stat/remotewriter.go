package stat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/natuleadan/sdk-api/infra/logx"
)

const (
	httpTimeout     = time.Second * 5
	jsonContentType = "application/json; charset=utf-8"
)

// ErrWriteFailed is an error that indicates failed to submit a StatReport.
var ErrWriteFailed = errors.New("submit failed")

// A RemoteWriter is a writer to write StatReport.
type RemoteWriter struct {
	endpoint string
}

// NewRemoteWriter returns a RemoteWriter.
func NewRemoteWriter(endpoint string) Writer {
	return &RemoteWriter{
		endpoint: endpoint,
	}
}

func (rw *RemoteWriter) Write(report *StatReport) error {
	bs, err := json.Marshal(report)
	if err != nil {
		return err
	}

	client := &http.Client{
		Timeout: httpTimeout,
	}
	req, err := http.NewRequestWithContext(context.Background(), "POST", rw.endpoint, bytes.NewReader(bs))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", jsonContentType)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logx.Errorf("write report failed, code: %d, reason: %s", resp.StatusCode, resp.Status)
		return ErrWriteFailed
	}

	return nil
}
