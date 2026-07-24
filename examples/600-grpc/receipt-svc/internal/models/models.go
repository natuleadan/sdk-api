package models

type Receipt struct {
	ID          int64  `db:"id,primary,auto"`
	TransferID  string `db:"transfer_id,required"`
	FromAccount string `db:"from_account,required"`
	ToAccount   string `db:"to_account,required"`
	Amount      string `db:"amount,required"`
	StorageKey  string `db:"storage_key,required"`
}
