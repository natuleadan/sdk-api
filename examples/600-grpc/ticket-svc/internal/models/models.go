package models

type Ticket struct {
	ID    int64   `db:"id,primary,auto"`
	Name  string  `db:"name,required"`
	Price float64 `db:"price"`
	Stock int     `db:"stock,default=0"`
}

type Order struct {
	ID        int64  `db:"id,primary,auto"`
	TicketID  int64  `db:"ticket_id,required"`
	Quantity  int    `db:"quantity"`
	UserID    string `db:"user_id,required"`
	Status    string `db:"status,default='pending'"`
}
