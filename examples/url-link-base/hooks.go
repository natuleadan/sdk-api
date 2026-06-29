package main

import (
	"context"
	"crypto/rand"
	"math/big"

	"url-link-base/models"

	"github.com/natuleadan/sdk-api/runtime"
)

type LinkHooks struct {
	runtime.DefaultHooks[models.Link]
}

func (h *LinkHooks) BeforeCreate(ctx context.Context, req models.Link) (models.Link, error) {
	if req.ShortCode == "" {
		req.ShortCode = generateShortCode(8)
	}
	return req, nil
}

func generateShortCode(n int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	code := make([]byte, n)
	for i := range code {
		idx, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		code[i] = charset[idx.Int64()]
	}
	return string(code)
}
