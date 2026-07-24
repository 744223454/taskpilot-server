package logic

import "errors"

var (
	ErrDatabaseUnavailable = errors.New("database not connected")
	ErrInvalidInput        = errors.New("invalid input")
	ErrNotFound            = errors.New("resource not found")
	ErrConflict            = errors.New("resource conflict")
	ErrInvalidState        = errors.New("invalid resource state")
)
