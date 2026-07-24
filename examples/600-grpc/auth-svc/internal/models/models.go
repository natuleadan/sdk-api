package models

type User struct {
	ID       string `db:"id,primary,type=UUID,default=gen_random_uuid()"`
	Username string `db:"username,unique,required"`
	Password string `db:"password,required"`
	Role     string `db:"role,required,default='viewer'"`
	Credits  int    `db:"credits,default=15"`
}
