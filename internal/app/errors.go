package app

import "errors"

var ErrInvalidInput = errors.New("invalid input")

type invalidInputError struct {
	message string
}

func (e invalidInputError) Error() string {
	return e.message
}

func (e invalidInputError) Is(target error) bool {
	return target == ErrInvalidInput
}

func newInvalidInputError(message string) error {
	return invalidInputError{message: message}
}
