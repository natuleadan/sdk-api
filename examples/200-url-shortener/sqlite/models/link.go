package models

type Link struct {
	ID        int64  `db:"id,primary,auto"        json:"id"`
	ShortCode string `db:"short_code,unique"       json:"shortCode"`
	TargetURL string `db:"target_url,required"      json:"targetUrl"`
}
