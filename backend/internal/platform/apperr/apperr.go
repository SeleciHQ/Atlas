// Package apperr defines typed application errors and their public problem
// representation. Internal causes are never exposed in API responses.
package apperr

import (
	"errors"
	"fmt"
	"net/http"
)

// Kind classifies an application error for transport mapping.
type Kind string

const (
	KindInvalid         Kind = "invalid"
	KindUnauthenticated Kind = "unauthenticated"
	KindForbidden       Kind = "forbidden"
	KindNotFound        Kind = "not_found"
	KindConflict        Kind = "conflict"
	KindUnavailable     Kind = "unavailable"
	KindInternal        Kind = "internal"
)

// Error is a typed application error.
type Error struct {
	Kind    Kind
	Code    string
	Message string
	Fields  map[string]string
	cause   error
}

// New creates an application error. Message is safe to return to clients.
func New(kind Kind, code, message string) *Error {
	return &Error{Kind: kind, Code: code, Message: message}
}

// WithField adds a field-level validation message.
func (e *Error) WithField(field, message string) *Error {
	if e.Fields == nil {
		e.Fields = map[string]string{}
	}
	e.Fields[field] = message
	return e
}

// WithCause attaches an internal cause. The cause is never returned to clients.
func (e *Error) WithCause(err error) *Error {
	e.cause = err
	return e
}

func (e *Error) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error { return e.cause }

// Problem is the HTTP problem document returned for errors.
type Problem struct {
	Type      string            `json:"type"`
	Title     string            `json:"title"`
	Status    int               `json:"status"`
	Code      string            `json:"code"`
	Detail    string            `json:"detail,omitempty"`
	Errors    map[string]string `json:"errors,omitempty"`
	RequestID string            `json:"request_id,omitempty"`
}

// From maps an error to a public problem document.
func From(err error) Problem {
	var appErr *Error
	if errors.As(err, &appErr) {
		status, title := statusFor(appErr.Kind)
		detail := appErr.Message
		code := appErr.Code
		if appErr.Kind == KindInternal {
			detail = "An unexpected error occurred."
			code = "internal_error"
		}
		if code == "" {
			code = string(appErr.Kind)
		}
		return Problem{
			Type:   "about:blank",
			Title:  title,
			Status: status,
			Code:   code,
			Detail: detail,
			Errors: appErr.Fields,
		}
	}
	return Problem{
		Type:   "about:blank",
		Title:  "Internal Server Error",
		Status: http.StatusInternalServerError,
		Code:   "internal_error",
		Detail: "An unexpected error occurred.",
	}
}

// IsInternal reports whether the error should be logged as a server fault.
func IsInternal(err error) bool {
	var appErr *Error
	if errors.As(err, &appErr) {
		return appErr.Kind == KindInternal
	}
	return true
}

func statusFor(kind Kind) (int, string) {
	switch kind {
	case KindInvalid:
		return http.StatusBadRequest, "Bad Request"
	case KindUnauthenticated:
		return http.StatusUnauthorized, "Unauthorized"
	case KindForbidden:
		return http.StatusForbidden, "Forbidden"
	case KindNotFound:
		return http.StatusNotFound, "Not Found"
	case KindConflict:
		return http.StatusConflict, "Conflict"
	case KindUnavailable:
		return http.StatusServiceUnavailable, "Service Unavailable"
	default:
		return http.StatusInternalServerError, "Internal Server Error"
	}
}
