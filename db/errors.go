package db

import "errors"

var (
	ErrNotStruct    = errors.New("model must be a struct")
	ErrNoPrimaryKey = errors.New("model must have a primary key field tagged with db:\"...primary\"")
	ErrNotFound     = errors.New("record not found")
)
