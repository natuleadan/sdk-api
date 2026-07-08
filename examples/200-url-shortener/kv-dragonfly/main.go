package main

import (
	"crypto/rand"
	"encoding/json"
	"log"
	"math/big"
	"os"
	"strconv"

	"github.com/natuleadan/sdk-api/infra/stores/redis"
	"github.com/natuleadan/sdk-api/runtime"
)

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

var rdb *redis.Redis

func getRedis() *redis.Redis {
	if rdb != nil {
		return rdb
	}
	addr := os.Getenv("DRAGONFLY_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	rdb = redis.MustNewRedis(redis.RedisConf{Host: addr, Type: redis.NodeType})
	log.Println("dragonfly ready")
	return rdb
}

type linkData struct {
	ID        int    `json:"id"`
	ShortCode string `json:"shortCode"`
	TargetURL string `json:"targetUrl"`
}

type linkBody struct {
	TargetURL string `json:"targetUrl"`
	ShortCode string `json:"shortCode,omitempty"`
}

func generateShortCode(n int) string {
	code := make([]byte, n)
	for i := range code {
		idx, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		code[i] = charset[idx.Int64()]
	}
	return string(code)
}

func main() {
	cfgPath := os.Getenv("CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = "service.yaml"
	}
	svc, err := runtime.New(cfgPath)
	if err != nil {
		log.Fatalf("init: %v", err)
	}

	svc.WithRest("createLink", func(c *runtime.RestCtx) error {
		var body linkBody
		if err := json.Unmarshal(c.Body(), &body); err != nil {
			return c.Status(400).JSON(map[string]any{"error": "invalid body"})
		}
		r := getRedis()
		code := body.ShortCode
		if code == "" {
			code = generateShortCode(8)
		}
		nextID, err := r.IncrCtx(c.Context(), "link:next_id")
		if err != nil {
			return c.Status(500).JSON(map[string]any{"error": err.Error()})
		}
		id := int(nextID)
		data := linkData{ID: id, ShortCode: code, TargetURL: body.TargetURL}
		b, _ := json.Marshal(data)
		if err := r.SetexCtx(c.Context(), "link:id:"+strconv.Itoa(id), string(b), 0); err != nil {
			return c.Status(500).JSON(map[string]any{"error": err.Error()})
		}
		if err := r.SetexCtx(c.Context(), "link:sc:"+code, string(b), 0); err != nil {
			return c.Status(500).JSON(map[string]any{"error": err.Error()})
		}
		return c.Status(201).JSON(data)
	})

	svc.WithRest("listLinks", func(c *runtime.RestCtx) error {
		r := getRedis()
		keys, err := r.KeysCtx(c.Context(), "link:id:*")
		if err != nil {
			return c.Status(500).JSON(map[string]any{"error": err.Error()})
		}
		var results []linkData
		for _, key := range keys {
			val, err := r.GetCtx(c.Context(), key)
			if err == nil {
				var d linkData
				if json.Unmarshal([]byte(val), &d) == nil {
					results = append(results, d)
				}
			}
		}
		if results == nil {
			results = []linkData{}
		}
		return c.JSON(results)
	})

	svc.WithRest("getLink", func(c *runtime.RestCtx) error {
		id := c.Params("id")
		r := getRedis()
		val, err := r.GetCtx(c.Context(), "link:id:"+id)
		if err != nil {
			return c.Status(404).JSON(map[string]any{"error": "not found"})
		}
		var d linkData
		json.Unmarshal([]byte(val), &d)
		return c.JSON(d)
	})

	svc.WithRest("updateLink", func(c *runtime.RestCtx) error {
		id := c.Params("id")
		var body linkBody
		if err := json.Unmarshal(c.Body(), &body); err != nil {
			return c.Status(400).JSON(map[string]any{"error": "invalid body"})
		}
		r := getRedis()
		val, err := r.GetCtx(c.Context(), "link:id:"+id)
		if err != nil {
			return c.Status(404).JSON(map[string]any{"error": "not found"})
		}
		var existing linkData
		json.Unmarshal([]byte(val), &existing)
		if body.TargetURL != "" {
			existing.TargetURL = body.TargetURL
		}
		b, _ := json.Marshal(existing)
		oldSC := existing.ShortCode
		if body.ShortCode != "" {
			r.DelCtx(c.Context(), "link:sc:"+oldSC)
			existing.ShortCode = body.ShortCode
		}
		b, _ = json.Marshal(existing)
		if err := r.SetexCtx(c.Context(), "link:id:"+id, string(b), 0); err != nil {
			return c.Status(500).JSON(map[string]any{"error": err.Error()})
		}
		if err := r.SetexCtx(c.Context(), "link:sc:"+existing.ShortCode, string(b), 0); err != nil {
			return c.Status(500).JSON(map[string]any{"error": err.Error()})
		}
		return c.JSON(existing)
	})

	svc.WithRest("deleteLink", func(c *runtime.RestCtx) error {
		id := c.Params("id")
		r := getRedis()
		val, err := r.GetCtx(c.Context(), "link:id:"+id)
		if err != nil {
			return c.Status(404).JSON(map[string]any{"error": "not found"})
		}
		var existing linkData
		json.Unmarshal([]byte(val), &existing)
		r.DelCtx(c.Context(), "link:id:"+id)
		r.DelCtx(c.Context(), "link:sc:"+existing.ShortCode)
		return c.SendStatus(204)
	})

	svc.WithRest("expandLink", func(c *runtime.RestCtx) error {
		code := c.Params("shortCode")
		r := getRedis()
		val, err := r.GetCtx(c.Context(), "link:sc:"+code)
		if err != nil {
			return c.Status(404).JSON(map[string]any{"error": "not found"})
		}
		var d linkData
		json.Unmarshal([]byte(val), &d)
		return c.JSON(d)
	})

	log.Fatal(svc.Run())
}
