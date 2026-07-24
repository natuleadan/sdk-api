package models

type Account struct {
	ID       int64   `db:"id,primary,auto"`
	UserID   string  `db:"user_id,required"`
	Currency string  `db:"currency,required"`
	Balance  float64 `db:"balance"`
	Status   string  `db:"status,required"`
}

type Transaction struct {
	ID             int64   `db:"id,primary,auto"`
	AccountID      int64   `db:"account_id,required"`
	Type           string  `db:"type,required"`
	Amount         float64 `db:"amount"`
	ReferenceType  string  `db:"reference_type"`
	ReferenceID    string  `db:"reference_id"`
	IdempotencyKey string  `db:"idempotency_key,unique"`
}
