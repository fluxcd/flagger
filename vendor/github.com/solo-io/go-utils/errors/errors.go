package errors

import (
	"strings"

	"github.com/pkg/errors"
)

func Wrapf(err error, format string, args ...interface{}) error {
	return errors.Wrapf(err, format, args...)
}

func Errorf(format string, args ...interface{}) error {
	return errors.Errorf(format, args...)
}

func Errors(msgs []string) error {
	return errors.Errorf(strings.Join(msgs, "\n"))
}
