// Package httpx provides the shared JSON envelope, typed errors and request
// decoding used by every admin controller — identical wire format to the
// participant API so the two frontends behave consistently.
package httpx

import (
	"encoding/json"
	"errors"
	"net/http"
)

// APIError carries an HTTP status and a stable machine code.
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

// Reusable error constructors.
var (
	ErrBadRequest   = func(m string) *APIError { return NewError(http.StatusBadRequest, "bad_request", m) }
	ErrUnauthorized = func(m string) *APIError { return NewError(http.StatusUnauthorized, "unauthorized", m) }
	ErrForbidden    = func(m string) *APIError { return NewError(http.StatusForbidden, "forbidden", m) }
	ErrNotFound     = func(m string) *APIError { return NewError(http.StatusNotFound, "not_found", m) }
	ErrConflict     = func(m string) *APIError { return NewError(http.StatusConflict, "conflict", m) }
	ErrInternal     = func(m string) *APIError { return NewError(http.StatusInternalServerError, "internal", m) }
)

type envelope struct {
	Data any `json:"data"`
}
type errorBody struct {
	Error *APIError `json:"error"`
}

// JSON writes a success envelope.
func JSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(envelope{Data: data})
}

// Error writes an error envelope; non-APIErrors become 500 to avoid leaks.
func Error(w http.ResponseWriter, err error) {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		apiErr = ErrInternal("something went wrong")
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(apiErr.Status)
	_ = json.NewEncoder(w).Encode(errorBody{Error: apiErr})
}

// Decode reads a JSON body, rejecting unknown fields.
func Decode(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return ErrBadRequest("invalid JSON body: " + err.Error())
	}
	return nil
}
