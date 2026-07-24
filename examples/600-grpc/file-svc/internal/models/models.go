package models

type FileRecord struct {
	ID          int64  `db:"id,primary,auto"`
	Filename    string `db:"filename,required"`
	Size        int64  `db:"size,default=0"`
	ContentType string `db:"content_type"`
	UserID      string `db:"user_id,required"`
	StorageKey  string `db:"storage_key,required,unique"`
}
