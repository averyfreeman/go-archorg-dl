package archorgdl

import (
	"errors"
	"fmt"
)

var (
	ErrOutputExists = errors.New("output file already exists")
	ErrNoVideo      = errors.New("Archive.org item has no downloadable video file")
	ErrNoDuration   = errors.New("program duration is unavailable for segmented fallback")
)

type Error struct {
	Op  string
	Err error
}

func (e *Error) Error() string {
	if e.Op == "" {
		return e.Err.Error()
	}
	return fmt.Sprintf("%s: %v", e.Op, e.Err)
}

func (e *Error) Unwrap() error { return e.Err }

func wrapError(op string, err error) error {
	if err == nil {
		return nil
	}
	return &Error{Op: op, Err: err}
}
