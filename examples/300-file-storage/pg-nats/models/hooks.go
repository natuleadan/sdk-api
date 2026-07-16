package models

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/natuleadan/sdk-api/runtime"
	"github.com/natuleadan/sdk-api/server"
)

type ProductHooks struct {
	runtime.DefaultHooks[Product]
	Store server.StorageBackend
}

func (h *ProductHooks) AfterCreate(ctx context.Context, entity *Product) error {
	log.Printf("Product created: ID=%d Name=%s MediaKey=%s", entity.ID, entity.Name, entity.MediaKey)
	return nil
}

func (h *ProductHooks) AfterGet(ctx context.Context, entity *Product) error {
	log.Printf("Product retrieved: ID=%d Name=%s", entity.ID, entity.Name)
	return nil
}

func (h *ProductHooks) AfterDelete(ctx context.Context, id string) error {
	log.Printf("Product deleted: ID=%s — S3 cleanup if media_key exists", id)
	// En un hook real, cargaríamos el producto y eliminaríamos de S3:
	// if entity.MediaKey != "" {
	//     h.Store.Delete(ctx, "uploads/" + entity.MediaKey)
	// }
	return nil
}

func TransformToPublic(p *Product, presigner server.Presigner) (*ProductPublic, error) {
	pub := &ProductPublic{
		ID:    p.ID,
		Name:  p.Name,
		Price: p.Price,
	}
	if p.MediaKey != "" && presigner != nil {
		ttl := presignTTL(presigner)
		url, err := presigner.PresignURL(context.Background(), fmt.Sprintf("uploads/%s", p.MediaKey), ttl)
		if err != nil {
			log.Printf("presign %s: %v", p.MediaKey, err)
			return pub, nil
		}
		pub.MediaURL = url
	}
	return pub, nil
}

func presignTTL(p any) time.Duration {
	if pt, ok := p.(interface{ PresignTTL() time.Duration }); ok {
		return pt.PresignTTL()
	}
	return 5 * time.Minute
}
