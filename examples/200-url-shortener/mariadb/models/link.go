package models

import (
	"context"
	"crypto/rand"
	"math/big"

	"github.com/natuleadan/sdk-api/runtime"
)

type Link struct {
	ID        int64  `db:"id,primary,auto"  json:"id"`
	ShortCode string `db:"short_code,unique" json:"shortCode"`
	TargetURL string `db:"target_url,required" json:"targetUrl"`
}

type LinkHooks struct {
	runtime.DefaultHooks[Link]
}

func (h *LinkHooks) BeforeCreate(ctx context.Context, req Link) (Link, error) {
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
