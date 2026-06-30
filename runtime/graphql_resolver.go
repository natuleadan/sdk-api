package runtime

import (
	"encoding/json"

	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"
)

func callCRUDGet(provider CRUDProvider, id string) (any, error) {
	app := fiber.New()
	fctx := app.AcquireCtx(&fasthttp.RequestCtx{})
	defer app.ReleaseCtx(fctx)
	fctx.Request().Header.SetMethod("GET")
	if err := provider.Get(fctx, id); err != nil {
		return nil, err
	}
	return parseCRUDResponse(fctx)
}

func callCRUDList(provider CRUDProvider, page, size int, sort string) (any, error) {
	app := fiber.New()
	fctx := app.AcquireCtx(&fasthttp.RequestCtx{})
	defer app.ReleaseCtx(fctx)
	fctx.Request().Header.SetMethod("GET")
	params := ListParams{Page: page, Size: size, Sort: sort}
	if err := provider.List(fctx, params); err != nil {
		return nil, err
	}
	body := fctx.Response().Body()
	if len(body) > 0 {
		var wrapper struct {
			Data  any   `json:"data"`
			Total int64 `json:"total"`
		}
		if err := json.Unmarshal(body, &wrapper); err != nil {
			return string(body), nil
		}
		if wrapper.Data != nil {
			return wrapper.Data, nil
		}
		var result any
		if err := json.Unmarshal(body, &result); err != nil {
			return string(body), nil
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
	if err := provider.Create(fctx, body); err != nil {
		return nil, err
	}
	return parseCRUDResponse(fctx)
}

func callCRUDUpdate(provider CRUDProvider, id string, input any) (any, error) {
	app := fiber.New()
	fctx := app.AcquireCtx(&fasthttp.RequestCtx{})
	defer app.ReleaseCtx(fctx)
	fctx.Request().Header.SetMethod("PATCH")
	fctx.Request().Header.Set("Content-Type", "application/json")
	body, _ := json.Marshal(input)
	fctx.Request().SetBody(body)
	if err := provider.Update(fctx, id, body); err != nil {
		return nil, err
	}
	return parseCRUDResponse(fctx)
}

func callCRUDDelete(provider CRUDProvider, id string) error {
	app := fiber.New()
	fctx := app.AcquireCtx(&fasthttp.RequestCtx{})
	defer app.ReleaseCtx(fctx)
	fctx.Request().Header.SetMethod("DELETE")
	return provider.Delete(fctx, id)
}

func parseCRUDResponse(c *fiber.Ctx) (any, error) {
	body := c.Response().Body()
	if len(body) > 0 {
		var result any
		if err := json.Unmarshal(body, &result); err != nil {
			return string(body), nil
		}
		return result, nil
	}
	return nil, nil
}
