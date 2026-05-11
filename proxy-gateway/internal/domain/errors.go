package domain

import "errors"

var (
	ErrAPIKeyNotFound    = errors.New("api key not found")
	ErrAPIKeyRevoked     = errors.New("api key revoked")
	ErrTargetMissing     = errors.New("X-Echo-Target header missing")
	ErrTargetInvalid     = errors.New("target url invalid")
	ErrTargetNotAllowed  = errors.New("target host not in api key allowlist")
	ErrTargetUnsafe      = errors.New("target host blocked (private or loopback)")
)
