package models

type Product struct {
	ID       int64   `db:"id,primary,auto" json:"id"`
	Name     string  `db:"name,required" json:"name"`
	Price    float64 `db:"price" json:"price"`
	MediaKey string  `db:"media_key" json:"mediaKey,omitempty"`
}

type ProductPublic struct {
	ID       int64   `json:"id"`
	Name     string  `json:"name"`
	Price    float64 `json:"price"`
	MediaURL string  `json:"mediaURL,omitempty"`
}

type UploadResponse struct {
	Key        string `json:"key"`
	Size       int    `json:"size"`
	PresignURL string `json:"presignURL,omitempty"`
}


