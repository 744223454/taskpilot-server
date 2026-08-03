package logic

import "errors"

var (
	ErrDatabaseUnavailable = errors.New("database not connected")
	ErrCacheUnavailable    = errors.New("cache unavailable")
	ErrInvalidInput        = errors.New("invalid input")
	ErrNotFound            = errors.New("resource not found")
	ErrConflict            = errors.New("resource conflict")
	ErrInvalidState        = errors.New("invalid resource state")
	ErrPayloadTooLarge     = errors.New("uploaded file too large")
	ErrUnsupportedFileType = errors.New("unsupported file type")
	ErrPDFUnprocessable    = errors.New("PDF cannot be processed")
	ErrExtractionBusy      = errors.New("PDF extraction service is busy")
	ErrRateLimited         = errors.New("too many requests")
)
