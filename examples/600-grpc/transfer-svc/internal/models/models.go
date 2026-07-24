package models

type Transfer struct {
	ID             int64   `db:"id,primary,auto"`
	FromAccountID  int64   `db:"from_account_id,required"`
	ToAccountID    int64   `db:"to_account_id,required"`
	Amount         float64 `db:"amount"`
	Currency       string  `db:"currency,required"`
	Status         string  `db:"status,required"`
	IdempotencyKey string  `db:"idempotency_key,unique"`
}
