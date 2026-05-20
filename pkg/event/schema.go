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
// Matches the service name used everywhere else (docker compose service,
// k8s deployment, Kafka client ID) and the value the dashboard ModeBadge
// expects. Legacy events stored with source="proxy" are still recognized
// downstream — see log-consumer and dashboard ModeBadge.
const SourceProxy = "proxy-gateway"

// SourceProxyLegacy is the old source value emitted by pre-0.4 proxy-gateway
// builds. Kept so consumers (log-consumer, analytics) can recognize old rows
// in ClickHouse without a data migration.
const SourceProxyLegacy = "proxy"

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
