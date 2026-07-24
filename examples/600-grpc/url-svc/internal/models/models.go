package models

type Link struct {
	ID        int64  `db:"id,primary,auto"`
	ShortCode string `db:"short_code,required,unique"`
	TargetURL string `db:"target_url,required"`
	UserID    string `db:"user_id,required"`
}
