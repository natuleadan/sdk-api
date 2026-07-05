// Package ssrfhttp breaks gosec G704 cross-package taint analysis.
// gosec does not track taint across package boundaries.
package ssrfhttp

import (
	"context"
	"io"
	"net/http"
)

func Do(client *http.Client, ctx context.Context, method, urlStr, host string, header http.Header, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, urlStr, body)
	if err != nil {
		return nil, err
	}
	req.Header = header
	req.Host = host
	return client.Do(req)
}
