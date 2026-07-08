//go:build tursoinit

package main

import (
	"database/sql"
	"fmt"
	"os"

	_ "turso.tech/database/tursogo"
)

func main() {
	path := os.Args[1]
	db, err := sql.Open("turso", path)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer db.Close()

	_, err = db.Exec("CREATE TABLE IF NOT EXISTS turso_init (a text)")
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("TURSO OK")
}
