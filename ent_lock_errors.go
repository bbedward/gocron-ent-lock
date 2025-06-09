package entlock

import "errors"

var (
	ErrEntCantBeNull    = errors.New("ent client can't be null")
	ErrWorkerIsRequired = errors.New("worker is required")
	ErrStatusIsRequired = errors.New("status is required")
)