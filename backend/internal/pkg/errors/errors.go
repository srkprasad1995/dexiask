// Package errors provides application error types with a code that maps to an
// HTTP status. No gRPC coupling.
package errors

import (
	"errors"
	"fmt"
	"net/http"
)

// ErrorCode is an application-specific error classification.
type ErrorCode string

const (
	CodeInternal        ErrorCode = "INTERNAL_ERROR"
	CodeNotFound        ErrorCode = "NOT_FOUND"
	CodeInvalidArgument ErrorCode = "INVALID_ARGUMENT"
	CodeUnavailable     ErrorCode = "UNAVAILABLE"
)

// AppError represents an application error with a classification code.
type AppError struct {
	Code    ErrorCode
	Message string
	Err     error
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *AppError) Unwrap() error { return e.Err }

func newAppError(code ErrorCode, message string, err error) *AppError {
	return &AppError{Code: code, Message: message, Err: err}
}

func NotFound(message string) *AppError { return newAppError(CodeNotFound, message, nil) }
func NotFoundf(format string, args ...interface{}) *AppError {
	return newAppError(CodeNotFound, fmt.Sprintf(format, args...), nil)
}
func InvalidArgument(message string) *AppError { return newAppError(CodeInvalidArgument, message, nil) }
func InvalidArgumentf(format string, args ...interface{}) *AppError {
	return newAppError(CodeInvalidArgument, fmt.Sprintf(format, args...), nil)
}
func Internal(message string, err error) *AppError { return newAppError(CodeInternal, message, err) }
func Unavailable(message string, err error) *AppError {
	return newAppError(CodeUnavailable, message, err)
}

// HTTPStatus maps an error to an HTTP status code, defaulting to 500.
func HTTPStatus(err error) int {
	var appErr *AppError
	if !errors.As(err, &appErr) {
		return http.StatusInternalServerError
	}
	switch appErr.Code {
	case CodeNotFound:
		return http.StatusNotFound
	case CodeInvalidArgument:
		return http.StatusBadRequest
	case CodeUnavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}
