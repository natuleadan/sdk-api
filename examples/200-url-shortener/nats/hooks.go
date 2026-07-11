package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"strconv"
	"sync"

	"url-shortener-nats/models"

	"github.com/natuleadan/sdk-api/runtime"
)

type LinkHooks struct {
	runtime.DefaultHooks[models.Link]
	codeByID map[int64]string
	codeMu   sync.RWMutex
}

func (h *LinkHooks) BeforeCreate(ctx context.Context, req models.Link) (models.Link, error) {
	if req.ShortCode == "" {
		req.ShortCode = generateShortCode(8)
	}
	return req, nil
}

func (h *LinkHooks) AfterCreate(ctx context.Context, created *models.Link) error {
	if created == nil {
		return nil
	}
	h.codeMu.Lock()
	h.codeByID[created.ID] = created.ShortCode
	h.codeMu.Unlock()

	ensureEventSvc()
	if eventSvc == nil {
		return nil
	}
	return eventSvc.PublishJSON(ctx, "links.created", models.URLEvent{
		Type:      "created",
		LinkID:    created.ID,
		ShortCode: created.ShortCode,
		TargetURL: created.TargetURL,
	})
}

func (h *LinkHooks) AfterUpdate(ctx context.Context, updated *models.Link) error {
	if updated == nil {
		return nil
	}
	h.codeMu.Lock()
	h.codeByID[updated.ID] = updated.ShortCode
	h.codeMu.Unlock()

	// Invalidate both cache entries inline (not async via worker)
	idKey := "id." + strconv.FormatInt(updated.ID, 10)
	scKey := "sc." + updated.ShortCode
	if conn := ensureCacheConn(); conn != nil {
		if err := conn.KVDelete("url-expand-cache", idKey); err != nil {
			fmt.Printf("cache del %s: %v\n", idKey, err)
		}
		if err := conn.KVDelete("url-expand-cache", scKey); err != nil {
			fmt.Printf("cache del %s: %v\n", scKey, err)
		}
	}

	ensureEventSvc()
	if eventSvc == nil {
		return nil
	}
	return eventSvc.PublishJSON(ctx, "links.updated", models.URLEvent{
		Type:      "updated",
		LinkID:    updated.ID,
		ShortCode: updated.ShortCode,
		TargetURL: updated.TargetURL,
	})
}

func (h *LinkHooks) AfterDelete(ctx context.Context, id string) error {
	h.codeMu.RLock()
	shortCode, ok := h.codeByID[int64(parseID(id))]
	h.codeMu.RUnlock()

	// Invalidate both cache entries inline
	idKey := "id." + id
	if conn := ensureCacheConn(); conn != nil {
		if err := conn.KVDelete("url-expand-cache", idKey); err != nil {
			fmt.Printf("cache del %s: %v\n", idKey, err)
		}
		if ok && shortCode != "" {
			scKey := "sc." + shortCode
			if err := conn.KVDelete("url-expand-cache", scKey); err != nil {
				fmt.Printf("cache del %s: %v\n", scKey, err)
			}
		}
	}

	ensureEventSvc()
	if eventSvc == nil {
		return nil
	}
	evt := models.URLEvent{Type: "deleted", LinkID: parseID(id)}
	if ok {
		evt.ShortCode = shortCode
	}
	return eventSvc.PublishJSON(ctx, "links.deleted", evt)
}

func parseID(s string) int64 {
	var id int64
	for _, c := range s {
		if c >= '0' && c <= '9' {
			id = id*10 + int64(c-'0')
		} else {
			break
		}
	}
	return id
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
