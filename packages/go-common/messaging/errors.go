package messaging

import (
	"errors"
	"fmt"
)

type permanentError struct {
	err error
}

func (e *permanentError) Error() string {
	return fmt.Sprintf("permanent: %v", e.err)
}

func (e *permanentError) Unwrap() error {
	return e.err
}

func Permanent(err error) error {
	if err == nil {
		return nil
	}
	if IsPermanent(err) {
		return err
	}
	return &permanentError{err: err}
}

func IsPermanent(err error) bool {
	var target *permanentError
	return errors.As(err, &target)
}
