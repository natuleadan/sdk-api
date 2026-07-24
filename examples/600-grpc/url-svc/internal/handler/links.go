package handler

import (
	authpb "600-grpc/pb/authpb"
	"600-grpc/url-svc/internal/models"

	"github.com/natuleadan/sdk-api/db"
	"github.com/natuleadan/sdk-api/runtime"
	"github.com/natuleadan/sdk-api/server/middleware"
)

type ServiceContext struct {
	svc   *runtime.Service
	links *db.Table[models.Link]
}

func NewServiceContext(s *runtime.Service, links *db.Table[models.Link]) *ServiceContext {
	return &ServiceContext{svc: s, links: links}
}

func CreateLink(svcCtx *ServiceContext) func(*runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		var body struct {
			TargetURL string `json:"target_url"`
		}
		if err := c.Bind(&body); err != nil || body.TargetURL == "" {
			return c.Status(400).JSON(runtime.Map{"error": "invalid body"})
		}
		a := c.Locals("auth").(*middleware.AuthContext)

		gc := svcCtx.svc.GetGRPCClient("auth-svc")
		if gc == nil {
			return c.Status(500).JSON(runtime.Map{"error": "gRPC client not available"})
		}
		authClient := authpb.NewAuthServiceClient(gc.Conn())
		cr, err := authClient.DeductCredit(c.Context(), &authpb.DeductCreditRequest{UserId: a.UserID, Amount: 1})
		if err != nil {
			return c.Status(500).JSON(runtime.Map{"error": "gRPC error: " + err.Error()})
		}
		if !cr.Ok {
			return c.Status(402).JSON(runtime.Map{"error": "insufficient credits"})
		}

		code := runtime.GenerateShortCode(8)
		link := models.Link{ShortCode: code, TargetURL: body.TargetURL, UserID: a.UserID}
		if err := svcCtx.links.Create(c.Context(), &link); err != nil {
			return c.Status(500).JSON(runtime.Map{"error": "create failed"})
		}
		return c.Status(201).JSON(runtime.Map{
			"id": link.ID, "short_code": code, "target_url": body.TargetURL,
		})
	}
}

func ExpandLink(svcCtx *ServiceContext) func(*runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		code := c.Params("code")
		link, err := svcCtx.links.FindBy(c.Context(), "short_code", code)
		if err != nil {
			return c.Status(404).JSON(runtime.Map{"error": "not found"})
		}
		return c.JSON(runtime.Map{"target_url": link.TargetURL})
	}
}
