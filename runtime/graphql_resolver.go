package runtime

import (
	"encoding/json"

	"github.com/gofiber/fiber/v3"
	"github.com/valyala/fasthttp"
)

// The callCRUD* helpers invoke CRUD providers out-of-band (GraphQL resolvers
// have no HTTP request). They fabricate a Fiber context, wrap it in a
// RestCtx like the HTTP boundary does, and read the written response back.
func callCRUDGet(provider CRUDProvider, id string) (any, error) {
	app := fiber.New()
	fctx := app.AcquireCtx(&fasthttp.RequestCtx{})
	defer app.ReleaseCtx(fctx)
	fctx.Request().Header.SetMethod("GET")
	rc := newRestCtx(fctx, nil)
	if err := provider.Get(rc, id); err != nil {
		return nil, err
	}
	return parseCRUDResponse(rc), nil
}

func callCRUDList(provider CRUDProvider, page, size int, sort string) (any, error) {
	app := fiber.New()
	fctx := app.AcquireCtx(&fasthttp.RequestCtx{})
	defer app.ReleaseCtx(fctx)
	fctx.Request().Header.SetMethod("GET")
	rc := newRestCtx(fctx, nil)
	params := ListParams{Page: page, Size: size, Sort: sort}
	if err := provider.List(rc, params); err != nil {
		return nil, err
	}
	body := rc.ResponseBody()
	if len(body) > 0 {
		var wrapper struct {
			Data  any   `json:"data"`
			Total int64 `json:"total"`
		}
		if err := json.Unmarshal([]byte(body), &wrapper); err != nil {
			return body, nil
		}
		if wrapper.Data != nil {
			return wrapper.Data, nil
		}
		var result any
		if err := json.Unmarshal([]byte(body), &result); err != nil {
			return body, nil
		}
		return result, nil
	}
	return []any{}, nil
}

func callCRUDCreate(provider CRUDProvider, input any) (any, error) {
	app := fiber.New()
	fctx := app.AcquireCtx(&fasthttp.RequestCtx{})
	defer app.ReleaseCtx(fctx)
	fctx.Request().Header.SetMethod("POST")
	fctx.Request().Header.Set("Content-Type", "application/json")
	body, _ := json.Marshal(input)
	fctx.Request().SetBody(body)
	rc := newRestCtx(fctx, nil)
	if err := provider.Create(rc, body); err != nil {
		return nil, err
	}
	return parseCRUDResponse(rc), nil
}

func callCRUDUpdate(provider CRUDProvider, id string, input any) (any, error) {
	app := fiber.New()
	fctx := app.AcquireCtx(&fasthttp.RequestCtx{})
	defer app.ReleaseCtx(fctx)
	fctx.Request().Header.SetMethod("PATCH")
	fctx.Request().Header.Set("Content-Type", "application/json")
	body, _ := json.Marshal(input)
	fctx.Request().SetBody(body)
	rc := newRestCtx(fctx, nil)
	if err := provider.Update(rc, id, body); err != nil {
		return nil, err
	}
	return parseCRUDResponse(rc), nil
}

func callCRUDDelete(provider CRUDProvider, id string) error {
	app := fiber.New()
	fctx := app.AcquireCtx(&fasthttp.RequestCtx{})
	defer app.ReleaseCtx(fctx)
	fctx.Request().Header.SetMethod("DELETE")
	return provider.Delete(newRestCtx(fctx, nil), id)
}

func parseCRUDResponse(c *RestCtx) any {
	body := c.ResponseBody()
	if len(body) > 0 {
		var result any
		if err := json.Unmarshal([]byte(body), &result); err != nil {
			return body
		}
		return result
	}
	return nil
}
