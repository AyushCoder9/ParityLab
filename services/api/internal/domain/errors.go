package domain

import "fmt"

type Error struct {
	Type       string `json:"type"`
	Code       string `json:"code"`
	Message    string `json:"message"`
	Param      string `json:"param,omitempty"`
	RequestID  string `json:"request_id"`
	HTTPStatus int    `json:"-"`
}

func (e *Error) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }

func Invalid(code, message, param string) *Error {
	return &Error{Type: "invalid_request_error", Code: code, Message: message, Param: param, HTTPStatus: 400}
}

func NotFound(resource, id string) *Error {
	return &Error{
		Type:       "invalid_request_error",
		Code:       resource + "_not_found",
		Message:    fmt.Sprintf("No %s exists with id %q.", resource, id),
		Param:      resource + "_id",
		HTTPStatus: 404,
	}
}

func Conflict(code, message, param string) *Error {
	return &Error{Type: "idempotency_error", Code: code, Message: message, Param: param, HTTPStatus: 409}
}
