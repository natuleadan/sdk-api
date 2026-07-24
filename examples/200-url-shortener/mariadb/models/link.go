package models

import (
	"context"

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
		req.ShortCode = runtime.GenerateShortCode(8)
	}
	return req, nil
}
