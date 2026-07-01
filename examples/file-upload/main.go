package main

import (
	"fmt"
	"io"
	"log"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/natuleadan/sdk-api/runtime"
)

func main() {
	svc, err := runtime.New("service.yaml")
	if err != nil {
		log.Fatalf("init: %v", err)
	}

	uploadDir := os.Getenv("UPLOAD_DIR")
	if uploadDir == "" {
		uploadDir = "/data/uploads"
	}
	os.MkdirAll(uploadDir, 0750)

	// Upload handler
	svc.WithRest("onUpload", func(c *fiber.Ctx) error {
		file, err := c.FormFile("file")
		if err != nil {
			return fiber.NewError(400, "missing file: "+err.Error())
		}

		src, err := file.Open()
		if err != nil {
			return fiber.NewError(500, "open file: "+err.Error())
		}
		defer src.Close()

		path := uploadDir + "/" + file.Filename
		dst, err := os.Create(path)
		if err != nil {
			return fiber.NewError(500, "save: "+err.Error())
		}
		defer dst.Close()

		if _, err := io.Copy(dst, src); err != nil {
			return fiber.NewError(500, "write: "+err.Error())
		}

		return c.JSON(fiber.Map{
			"filename": file.Filename,
			"size":     file.Size,
			"message":  "uploaded",
		})
	})

	// Download handler
	svc.WithRest("onDownload", func(c *fiber.Ctx) error {
		id := c.Params("id")
		path := uploadDir + "/" + id

		f, err := os.Open(path)
		if err != nil {
			return fiber.NewError(404, "file not found")
		}
		defer f.Close()

		c.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, id))
		return c.SendStream(f)
	})

	// File info handler
	svc.WithRest("onFileInfo", func(c *fiber.Ctx) error {
		id := c.Params("id")
		path := uploadDir + "/" + id

		info, err := os.Stat(path)
		if err != nil {
			return fiber.NewError(404, "file not found")
		}

		return c.JSON(fiber.Map{
			"filename": id,
			"size":     info.Size(),
			"modTime":  info.ModTime(),
		})
	})

	if err := svc.Run(); err != nil {
		log.Fatalf("run: %v", err)
	}
}
