package targetjob

import "errors"

var (
	ErrNotFound   = errors.New("target job not found")
	ErrValidation = errors.New("target job validation failed")
)
