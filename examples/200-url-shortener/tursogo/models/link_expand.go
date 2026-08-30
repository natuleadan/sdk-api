package models

type LinkExpand struct {
	ShortCode string `db:"short_code,primary" json:"shortCode"`
	TargetURL string `db:"target_url"         json:"targetUrl"`
}
