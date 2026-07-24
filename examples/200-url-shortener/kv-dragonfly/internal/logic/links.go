package logic

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/natuleadan/sdk-api/infra/stores/redis"
	"github.com/natuleadan/sdk-api/runtime"
)


type LinkData struct {
	ID        int    `json:"id"`
	ShortCode string `json:"shortCode"`
	TargetURL string `json:"targetUrl"`
}

type LinkBody struct {
	TargetURL string `json:"targetUrl"`
	ShortCode string `json:"shortCode,omitempty"`
}

type LinkLogic struct {
	rdb *redis.Redis
}

func NewLinkLogic(rdb *redis.Redis) *LinkLogic {
	return &LinkLogic{rdb: rdb}
}

func (l *LinkLogic) Create(ctx context.Context, body LinkBody) (*LinkData, error) {
	r := l.rdb
	code := body.ShortCode
	if code == "" {
		code = runtime.GenerateShortCode(8)
	}
	nextID, err := r.IncrCtx(ctx, "link:next_id")
	if err != nil {
		return nil, err
	}
	id := int(nextID)
	data := LinkData{ID: id, ShortCode: code, TargetURL: body.TargetURL}
	b, _ := json.Marshal(data)
	if err := r.SetexCtx(ctx, "link:id:"+strconv.Itoa(id), string(b), 0); err != nil {
		return nil, err
	}
	if err := r.SetexCtx(ctx, "link:sc:"+code, string(b), 0); err != nil {
		return nil, err
	}
	if _, err := r.DoCtx(ctx, "SADD", "link:ids", id); err != nil {
		return nil, err
	}
	return &data, nil
}

func (l *LinkLogic) List(ctx context.Context, page, size int) ([]LinkData, int64, error) {
	r := l.rdb
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}

	totalCmd, err := r.DoCtx(ctx, "SCARD", "link:ids")
	if err != nil {
		return nil, 0, err
	}
	total, _ := totalCmd.(int64)

	skip := (page - 1) * size
	scanned := 0
	cur := uint64(0)
	var ids []string
	for {
		res, err := r.DoCtx(ctx, "SSCAN", "link:ids", cur, "COUNT", 100)
		if err != nil {
			return nil, 0, err
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

	var results []LinkData
	for _, id := range ids {
		val, err := r.GetCtx(ctx, "link:id:"+id)
		if err == nil {
			var d LinkData
			if json.Unmarshal([]byte(val), &d) == nil {
				results = append(results, d)
			}
		}
	}
	if results == nil {
		results = []LinkData{}
	}
	return results, total, nil
}

func (l *LinkLogic) GetByShortCode(ctx context.Context, shortCode string) (*LinkData, error) {
	r := l.rdb
	val, err := r.GetCtx(ctx, "link:sc:"+shortCode)
	if err != nil {
		return nil, err
	}
	var d LinkData
	json.Unmarshal([]byte(val), &d)
	return &d, nil
}

func (l *LinkLogic) GetByID(ctx context.Context, id string) (*LinkData, error) {
	r := l.rdb
	val, err := r.GetCtx(ctx, "link:id:"+id)
	if err != nil {
		return nil, err
	}
	var d LinkData
	json.Unmarshal([]byte(val), &d)
	return &d, nil
}

func (l *LinkLogic) Update(ctx context.Context, id string, body LinkBody) (*LinkData, error) {
	r := l.rdb
	val, err := r.GetCtx(ctx, "link:id:"+id)
	if err != nil {
		return nil, err
	}
	var existing LinkData
	json.Unmarshal([]byte(val), &existing)
	if body.TargetURL != "" {
		existing.TargetURL = body.TargetURL
	}
	if body.ShortCode != "" {
		r.DelCtx(ctx, "link:sc:"+existing.ShortCode)
		existing.ShortCode = body.ShortCode
	}
	b, _ := json.Marshal(existing)
	if err := r.SetexCtx(ctx, "link:id:"+id, string(b), 0); err != nil {
		return nil, err
	}
	if err := r.SetexCtx(ctx, "link:sc:"+existing.ShortCode, string(b), 0); err != nil {
		return nil, err
	}
	return &existing, nil
}

func (l *LinkLogic) Delete(ctx context.Context, shortCode string) error {
	r := l.rdb
	d, err := l.GetByShortCode(ctx, shortCode)
	if err != nil {
		return err
	}
	r.DelCtx(ctx, "link:id:"+strconv.Itoa(d.ID))
	r.DelCtx(ctx, "link:sc:"+shortCode)
	r.DoCtx(ctx, "SREM", "link:ids", d.ID)
	return nil
}
