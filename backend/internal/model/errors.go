package model

import "fmt"

// ErrInvalidInput is returned by model Validate() methods for bad input.
func ErrInvalidInput(msg string) error {
	return fmt.Errorf("invalid input: %s", msg)
}
