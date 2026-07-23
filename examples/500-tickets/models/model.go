package models

import "time"

type Ticket struct {
	ID          int64     `db:"id,primary,auto" json:"id"`
	Name        string    `db:"name,required" json:"name"`
	Description string    `db:"description" json:"description"`
	Price       float64   `db:"price" json:"price"`
	Stock       int       `db:"stock" json:"stock"`
	CreatedAt   time.Time `db:"created_at" json:"created_at,omitempty"`
}

type Order struct {
	ID        int64     `db:"id,primary,auto" json:"id"`
	TicketID  int64     `db:"ticket_id,required" json:"ticket_id"`
	Quantity  int       `db:"quantity" json:"quantity"`
	Status    string    `db:"status" json:"status"`
	CreatedAt time.Time `db:"created_at" json:"created_at,omitempty"`
}

type OrderEvent struct {
	OrderID  int64  `json:"order_id"`
	TicketID int64  `json:"ticket_id"`
	Quantity int    `json:"quantity"`
	Status   string `json:"status"`
}
