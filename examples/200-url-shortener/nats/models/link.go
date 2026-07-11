package models

type Link struct {
	ID        int64  `db:"id,primary,auto"  json:"id"`
	ShortCode string `db:"short_code,unique" json:"shortCode"`
	TargetURL string `db:"target_url,required" json:"targetUrl"`
}

type LinkExpand struct {
	ID        int64  `db:"id,primary,auto"  json:"id"`
	ShortCode string `db:"short_code,unique" json:"shortCode"`
	TargetURL string `db:"target_url,required" json:"targetUrl"`
}

type URLEvent struct {
	Type      string `json:"type"`
	LinkID    int64  `json:"linkId"`
	ShortCode string `json:"shortCode"`
	TargetURL string `json:"targetUrl"`
}
