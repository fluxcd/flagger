package errors

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
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

type existErr struct {
	meta core.Metadata
}

func (err *existErr) Error() string {
	return fmt.Sprintf("%v exists", err.meta)
}

func NewExistErr(meta core.Metadata) *existErr {
	return &existErr{meta: meta}
}

func IsExist(err error) bool {
	switch err.(type) {
	case *existErr:
		return true
	}
	return false
}

type notExistErr struct {
	namespace string
	name      string
	err       error
}

func (err *notExistErr) Error() string {
	if err.err != nil {
		return fmt.Sprintf("%v.%v does not exist: %v", err.namespace, err.name, err.err)
	}
	return fmt.Sprintf("%v.%v does not exist", err.namespace, err.name)
}

func NewNotExistErr(namespace, name string, err ...error) *notExistErr {
	if len(err) > 0 {
		return &notExistErr{namespace: namespace, name: name, err: err[0]}
	}
	return &notExistErr{namespace: namespace, name: name}
}

func IsNotExist(err error) bool {
	switch err.(type) {
	case *notExistErr:
		return true
	}
	return false
}

type resourceVersionErr struct {
	namespace string
	name      string
	given     string
	expected  string
}

func (err *resourceVersionErr) Error() string {
	return fmt.Sprintf("invalid resource version %v.%v given %v, expected %v", err.namespace, err.name, err.given, err.expected)
}

func NewResourceVersionErr(namespace, name, given, expected string) *resourceVersionErr {
	return &resourceVersionErr{namespace: namespace, name: name, given: given, expected: expected}
}

func IsResourceVersion(err error) bool {
	switch err.(type) {
	case *resourceVersionErr:
		return true
	}
	return false
}
