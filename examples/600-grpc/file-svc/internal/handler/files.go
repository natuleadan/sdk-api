package handler

import (
	"bytes"
	"fmt"
	"io"

	"600-grpc/file-svc/internal/models"
	"600-grpc/pb/authpb"

	"github.com/natuleadan/sdk-api/db"
	"github.com/natuleadan/sdk-api/runtime"
	"github.com/natuleadan/sdk-api/server/middleware"
)

type ServiceContext struct {
	svc   *runtime.Service
	files *db.Table[models.FileRecord]
}

func NewServiceContext(s *runtime.Service, files *db.Table[models.FileRecord]) *ServiceContext {
	return &ServiceContext{svc: s, files: files}
}

func UploadFile(svcCtx *ServiceContext) func(*runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		a := c.Locals("auth").(*middleware.AuthContext)

		gc := svcCtx.svc.GetGRPCClient("auth-svc")
		if gc != nil {
			cr, err := authpb.NewAuthServiceClient(gc.Conn()).DeductCredit(c.Context(),
				&authpb.DeductCreditRequest{UserId: a.UserID, Amount: 1})
			if err != nil || !cr.Ok {
				return c.Status(402).JSON(runtime.Map{"error": "insufficient credits"})
			}
		}

		key := runtime.GenerateShortCode(16)
		store := svcCtx.svc.Storage("/files/upload")
		body := c.Body()

		if store != nil {
			if err := store.Upload(c.Context(), fmt.Sprintf("uploads/%s", key),
				bytes.NewReader(body), int64(len(body)), c.Get("Content-Type")); err != nil {
				return c.Status(500).JSON(runtime.Map{"error": "upload failed"})
			}
		}

		rec := models.FileRecord{Filename: key, UserID: a.UserID, StorageKey: key}
		if err := svcCtx.files.Create(c.Context(), &rec); err != nil {
			return c.Status(500).JSON(runtime.Map{"error": "save failed"})
		}

		return c.Status(201).JSON(runtime.Map{"id": rec.ID, "filename": key, "storage_key": key})
	}
}

func DownloadFile(svcCtx *ServiceContext) func(*runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		key := c.Params("key")
		store := svcCtx.svc.Storage("/files/upload")
		if store == nil {
			return c.Status(500).JSON(runtime.Map{"error": "storage unavailable"})
		}

		reader, err := store.Download(c.Context(), fmt.Sprintf("uploads/%s", key))
		if err != nil {
			return c.Status(404).JSON(runtime.Map{"error": "not found"})
		}
		data, _ := io.ReadAll(reader)
		return c.Status(200).SendString(string(data))
	}
}
