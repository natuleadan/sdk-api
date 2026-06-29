package models

type Product struct {
	ID    int64   `db:"id,primary,auto" json:"id"`
	Name  string  `db:"name,required" json:"name"`
	Price float64 `db:"price" json:"price"`
	Stock int     `db:"stock,default=0" json:"stock"`
}
