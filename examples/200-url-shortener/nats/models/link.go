package models

import (
	"context"
	"strconv"
	"sync"

	"github.com/natuleadan/sdk-api/runtime"
)

type HooksBridge interface {
	PublishEvent(ctx context.Context, evt URLEvent) error
	CacheDelete(idKey, scKey string)
}

type Link struct {
	ID        int64  `db:"id,primary,auto"  json:"id"`
	ShortCode string `db:"short_code,unique" json:"shortCode"`
	TargetURL string `db:"target_url,required" json:"targetUrl"`
}

type LinkExpand struct {
	ID        int64  `db:"id,primary,auto"  json:"id"`
	ShortCode string `db:"short_code,unique" json:"shortCode"`
	TargetURL string `db:"target_url,required" json:"targetUrl"`
}

type URLEvent struct {
	Type      string `json:"type"`
	LinkID    int64  `json:"linkId"`
	ShortCode string `json:"shortCode"`
	TargetURL string `json:"targetUrl"`
}

type LinkHooks struct {
	runtime.DefaultHooks[Link]
	CodeByID map[int64]string
	codeMu   sync.RWMutex
}

var hooksBridge HooksBridge

func SetHooksBridge(b HooksBridge) {
	hooksBridge = b
}

func (h *LinkHooks) BeforeCreate(ctx context.Context, req Link) (Link, error) {
	if req.ShortCode == "" {
		req.ShortCode = runtime.GenerateShortCode(8)
	}
	return req, nil
}

func (h *LinkHooks) AfterCreate(ctx context.Context, created *Link) error {
	if created == nil {
		return nil
	}
	h.codeMu.Lock()
	h.CodeByID[created.ID] = created.ShortCode
	h.codeMu.Unlock()

	if hooksBridge != nil {
		hooksBridge.PublishEvent(ctx, URLEvent{
			Type: "created", LinkID: created.ID,
			ShortCode: created.ShortCode, TargetURL: created.TargetURL,
		})
	}
	return nil
}

func (h *LinkHooks) AfterUpdate(ctx context.Context, updated *Link) error {
	if updated == nil {
		return nil
	}
	h.codeMu.Lock()
	h.CodeByID[updated.ID] = updated.ShortCode
	h.codeMu.Unlock()

	idKey := "id." + strconv.FormatInt(updated.ID, 10)
	scKey := "sc." + updated.ShortCode
	if hooksBridge != nil {
		hooksBridge.CacheDelete(idKey, scKey)
		hooksBridge.PublishEvent(ctx, URLEvent{
			Type: "updated", LinkID: updated.ID,
			ShortCode: updated.ShortCode, TargetURL: updated.TargetURL,
		})
	}
	return nil
}

func (h *LinkHooks) AfterDelete(ctx context.Context, id string) error {
	idStr := id
	h.codeMu.RLock()
	shortCode, ok := h.CodeByID[int64(parseID(idStr))]
	h.codeMu.RUnlock()

	idKey := "id." + idStr
	scKey := ""
	if ok {
		scKey = "sc." + shortCode
	}
	if hooksBridge != nil {
		hooksBridge.CacheDelete(idKey, scKey)
		evt := URLEvent{Type: "deleted", LinkID: parseID(idStr)}
		if ok {
			evt.ShortCode = shortCode
		}
		hooksBridge.PublishEvent(ctx, evt)
	}
	return nil
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
