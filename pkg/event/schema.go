package event

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

// NewEventID returns a 16-byte hex random id. Callers should prefer ULID where
// availble; this is a sane fallback for environments without an ULID lib.
func NewEventID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// NowNanos returns the wall-clock time in nanoseconds.
func NowNanos() int64 {
	return time.Now().UTC().UnixNano()
}

// SourceProxy is the value of HttpEvent.Source emitted by proxy-gateway.
const SourceProxy = "proxy"

// SourceSDKPrefix is the conventional prefix for SDK sources (sdk-go, sdk-laravel...).
const SourceSDKPrefix = "sdk-"

// Validate performs minimal sanity checks before producing. We deliberately
// keep this lenient: the wire is forward-compatible, so unknown fields are OK.
func Validate(ev *HttpEvent) error {
	if ev == nil {
		return errInvalid("nil event")
	}
	if ev.Method == "" {
		return errInvalid("method required")
	}
	if ev.Host == "" {
		return errInvalid("host required")
	}
	return nil
}

type validationError struct{ msg string }

func (e *validationError) Error() string { return "event invalid: " + e.msg }
func errInvalid(msg string) error        { return &validationError{msg: msg} }
