package domain

import "errors"

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrEmailTaken         = errors.New("email already in use")
	ErrUserNotFound       = errors.New("user not found")
	ErrProjectNotFound    = errors.New("project not found")
	ErrAPIKeyNotFound     = errors.New("api key not found")
	ErrForbidden          = errors.New("forbidden")
)
