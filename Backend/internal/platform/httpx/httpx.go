// Package httpx contains small HTTP helpers shared by every controller: a
// uniform JSON response envelope, error mapping, and request decoding. Keeping
// these in one place guarantees every endpoint speaks the same wire format,
// which makes the frontend and the OpenAPI contract simple and predictable.
package httpx

import (
	"encoding/json"
	"errors"
	"net/http"
)

// APIError is a typed error that carries an HTTP status code and a stable,
// machine-readable code the frontend can switch on.
type APIError struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *APIError) Error() string { return e.Message }

// NewError builds an APIError.
func NewError(status int, code, message string) *APIError {
	return &APIError{Status: status, Code: code, Message: message}
}

// Common, reusable errors.
var (
	ErrBadRequest   = func(msg string) *APIError { return NewError(http.StatusBadRequest, "bad_request", msg) }
	ErrUnauthorized = func(msg string) *APIError { return NewError(http.StatusUnauthorized, "unauthorized", msg) }
	ErrForbidden    = func(msg string) *APIError { return NewError(http.StatusForbidden, "forbidden", msg) }
	ErrNotFound     = func(msg string) *APIError { return NewError(http.StatusNotFound, "not_found", msg) }
	ErrConflict     = func(msg string) *APIError { return NewError(http.StatusConflict, "conflict", msg) }
	ErrInternal     = func(msg string) *APIError { return NewError(http.StatusInternalServerError, "internal", msg) }
)

// Envelope is the standard success wrapper: {"data": ...}.
type Envelope struct {
	Data any `json:"data"`
}

// errorBody is the standard error wrapper: {"error": {...}}.
type errorBody struct {
	Error *APIError `json:"error"`
}

// JSON writes a success response with the standard envelope.
func JSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Envelope{Data: data})
}

// Error writes an error response. Any non-APIError is treated as a 500 so we
// never leak internal error strings to clients.
func Error(w http.ResponseWriter, err error) {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		apiErr = ErrInternal("something went wrong")
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(apiErr.Status)
	_ = json.NewEncoder(w).Encode(errorBody{Error: apiErr})
}

// Decode reads and validates a JSON request body into dst. It rejects unknown
// fields to catch client mistakes early.
func Decode(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return ErrBadRequest("invalid JSON body: " + err.Error())
	}
	return nil
}
