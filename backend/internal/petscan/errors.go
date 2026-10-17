package petscan

import (
	"errors"
	"fmt"
)

// ErrInvalid matches errors caused by the query parameters (HTTP 400).
var ErrInvalid = errors.New("invalid query")

// ErrUpstream matches failures of the databases or remote services (HTTP 502).
var ErrUpstream = errors.New("upstream failure")

type classifiedError struct {
	err   error
	class error
}

func (e classifiedError) Error() string        { return e.err.Error() }
func (e classifiedError) Unwrap() error        { return e.err }
func (e classifiedError) Is(target error) bool { return target == e.class }

// invalidf returns an error for a bad query; the message is shown to the
// user as is.
func invalidf(format string, args ...any) error {
	return classifiedError{fmt.Errorf(format, args...), ErrInvalid}
}

// upstream marks err as a failure of a database or remote service.
func upstream(err error) error {
	if err == nil {
		return nil
	}
	return classifiedError{err, ErrUpstream}
}
