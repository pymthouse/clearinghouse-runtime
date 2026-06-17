package admin

import "fmt"

type AdminError struct {
	Message string
	Code    string
}

func (e *AdminError) Error() string { return e.Message }

type BackendUnreachableError struct{ AdminError }

func NewBackendUnreachableError(msg string) *BackendUnreachableError {
	return &BackendUnreachableError{AdminError{Message: msg, Code: "BACKEND_UNREACHABLE"}}
}

type ResourceNotFoundError struct{ AdminError }

func NewResourceNotFoundError(resource, id string) *ResourceNotFoundError {
	return &ResourceNotFoundError{AdminError{
		Message: fmt.Sprintf("%s not found: %s", resource, id),
		Code:    "NOT_FOUND",
	}}
}
