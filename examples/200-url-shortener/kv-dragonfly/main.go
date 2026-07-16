package main

import (
	"crypto/rand"
	"encoding/json"
	"log"
	"math/big"
	"os"
	"strconv"

	"github.com/natuleadan/sdk-api/runtime"
	"github.com/natuleadan/sdk-api/infra/stores/redis"
)

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

var rdb *redis.Redis

func getRedis(svc *runtime.Service) *redis.Redis {
	if rdb != nil {
		return rdb
	}
	rdb = svc.KV("kv-main")
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

	redis := getRedis(svc)

	svc.WithRest("createLink", func(c *runtime.RestCtx) error {
		var body linkBody
		if err := json.Unmarshal(c.Body(), &body); err != nil {
			return c.Status(400).JSON(map[string]any{"error": "invalid body"})
		}
		r := redis
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
		if _, err := r.DoCtx(c.Context(), "SADD", "link:ids", id); err != nil {
			return c.Status(500).JSON(map[string]any{"error": err.Error()})
		}
		return c.Status(201).JSON(data)
	})

	svc.WithRest("listLinks", func(c *runtime.RestCtx) error {
		r := redis
		page, _ := strconv.Atoi(c.Query("page", "1"))
		size, _ := strconv.Atoi(c.Query("size", "20"))
		if page < 1 {
			page = 1
		}
		if size < 1 || size > 100 {
			size = 20
		}

		// Get total count from set cardinality
		totalCmd, err := r.DoCtx(c.Context(), "SCARD", "link:ids")
		if err != nil {
			return c.Status(500).JSON(map[string]any{"error": err.Error()})
		}
		total, _ := totalCmd.(int64)

		// SSCAN to get paginated IDs from the set
		skip := (page - 1) * size
		scanned := 0
		cur := uint64(0)
		var ids []string
		for {
			res, err := r.DoCtx(c.Context(), "SSCAN", "link:ids", cur, "COUNT", size)
			if err != nil {
				return c.Status(500).JSON(map[string]any{"error": err.Error()})
			}
			arr := res.([]any)
			cur, _ = strconv.ParseUint(string(arr[0].([]byte)), 10, 64)
			elements := arr[1].([]any)
			for _, elem := range elements {
				if scanned >= skip+size {
					break
				}
				if scanned >= skip {
					ids = append(ids, string(elem.([]byte)))
				}
				scanned++
			}
			if cur == 0 || scanned >= skip+size {
				break
			}
		}

		var results []linkData
		for _, id := range ids {
			val, err := r.GetCtx(c.Context(), "link:id:"+id)
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
		return c.JSON(map[string]any{
			"data":  results,
			"total": total,
			"page":  page,
			"size":  size,
		})
	})

	svc.WithRest("getLink", func(c *runtime.RestCtx) error {
		id := c.Params("id")
		r := redis
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
		r := redis
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
		r := redis
		val, err := r.GetCtx(c.Context(), "link:id:"+id)
		if err != nil {
			return c.Status(404).JSON(map[string]any{"error": "not found"})
		}
		var existing linkData
		json.Unmarshal([]byte(val), &existing)
		r.DelCtx(c.Context(), "link:id:"+id)
		r.DelCtx(c.Context(), "link:sc:"+existing.ShortCode)
		r.DoCtx(c.Context(), "SREM", "link:ids", id)
		return c.SendStatus(204)
	})

	svc.WithRest("expandLink", func(c *runtime.RestCtx) error {
		code := c.Params("shortCode")
		r := redis
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
