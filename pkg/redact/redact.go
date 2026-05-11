// Package redact strips sensitive data (auth headers, tokens, secrets, PII) from
// HTTP events before they're persisted. It is shared across the SDK, the proxy,
// and the ingest service so the same rules apply at every layer (defense in depth).
//
// Industry inspiration: Sentry's data scrubber, Datadog's APM obfuscator,
// New Relic's high-security mode.
package redact

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
)

// Mask is the placeholder used wherever a value is redacted. We deliberately keep
// it short and recognisable so users can search for it in logs.
const Mask = "[REDACTED]"

// DefaultHeaderDenylist matches case-insensitively against header names. Values
// for any matching header are replaced with Mask.
var DefaultHeaderDenylist = []string{
	"authorization",
	"proxy-authorization",
	"cookie",
	"set-cookie",
	"x-api-key",
	"x-auth-token",
	"x-csrf-token",
	"x-xsrf-token",
	"x-session-token",
	"x-access-token",
	"x-echo-key",
	"x-forwarded-authorization",
	"www-authenticate",
}

// DefaultJSONFieldDenylist matches case-insensitively against JSON keys (at any
// depth). Matching values are replaced with Mask.
var DefaultJSONFieldDenylist = []string{
	"password", "passwd", "pwd",
	"secret", "client_secret",
	"token", "access_token", "refresh_token", "id_token", "session_token",
	"api_key", "apikey", "auth_token", "authorization",
	"private_key", "privatekey",
	"credit_card", "cardnumber", "card_number", "cvv", "cvc",
	"ssn",
}

// DefaultPatterns is a small set of regex patterns covering the most common
// shapes of secrets that show up in raw bodies. Each Replacer should preserve
// the surrounding non-secret context.
var DefaultPatterns = []Pattern{
	{Name: "jwt", Re: regexp.MustCompile(`eyJ[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}`)},
	{Name: "bearer", Re: regexp.MustCompile(`(?i)Bearer\s+[A-Za-z0-9._\-]{20,}`)},
	{Name: "aws_access_key", Re: regexp.MustCompile(`AKIA[0-9A-Z]{16}`)},
	{Name: "github_token", Re: regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{36,}`)},
	{Name: "stripe_live", Re: regexp.MustCompile(`sk_live_[A-Za-z0-9]{20,}`)},
	{Name: "stripe_test", Re: regexp.MustCompile(`sk_test_[A-Za-z0-9]{20,}`)},
	{Name: "google_api", Re: regexp.MustCompile(`AIza[0-9A-Za-z_\-]{35}`)},
	{Name: "slack_token", Re: regexp.MustCompile(`xox[baprs]-[A-Za-z0-9\-]{10,}`)},
	{Name: "credit_card", Re: regexp.MustCompile(`\b(?:\d[ -]*?){13,16}\b`)}, // crude; Luhn is applied below
}

// Pattern names a regex used to scrub raw byte slices.
type Pattern struct {
	Name string
	Re   *regexp.Regexp
}

// Rules is the complete configuration for a single redaction pass. Zero value
// is safe and applies the defaults. The struct is JSON-serializable so per-key
// rules can be persisted in Postgres and shipped to the cache.
type Rules struct {
	HeaderDenylist    []string  `json:"header_denylist,omitempty"`
	JSONFieldDenylist []string  `json:"json_field_denylist,omitempty"`
	Patterns          []Pattern `json:"-"` // not serialized — patterns are global
	DisableDefaults   bool      `json:"disable_defaults,omitempty"`
}

// FromJSON parses rules from a JSON byte slice (as stored in Postgres). Returns
// the zero value on empty input — callers can pass that directly to New.
func FromJSON(b []byte) (Rules, error) {
	var r Rules
	if len(b) == 0 || string(b) == "{}" || string(b) == "null" {
		return r, nil
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return r, err
	}
	return r, nil
}

// Redactor applies a set of rules. Construct once per service (cheap; holds
// compiled lookup tables) and call its methods on the hot path.
type Redactor struct {
	headerSet  map[string]struct{}
	jsonSet    map[string]struct{}
	patterns   []Pattern
}

// New constructs a Redactor merging the package defaults with the caller's rules
// (unless DisableDefaults is set).
func New(r Rules) *Redactor {
	headers := map[string]struct{}{}
	jsonFields := map[string]struct{}{}
	var patterns []Pattern

	if !r.DisableDefaults {
		for _, h := range DefaultHeaderDenylist {
			headers[h] = struct{}{}
		}
		for _, f := range DefaultJSONFieldDenylist {
			jsonFields[f] = struct{}{}
		}
		patterns = append(patterns, DefaultPatterns...)
	}
	for _, h := range r.HeaderDenylist {
		headers[strings.ToLower(h)] = struct{}{}
	}
	for _, f := range r.JSONFieldDenylist {
		jsonFields[strings.ToLower(f)] = struct{}{}
	}
	patterns = append(patterns, r.Patterns...)

	return &Redactor{headerSet: headers, jsonSet: jsonFields, patterns: patterns}
}

// Headers returns a copy of the input map with denylisted keys masked.
// The original map is not mutated.
func (r *Redactor) Headers(in map[string]string) map[string]string {
	if len(in) == 0 {
		return in
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		if _, hit := r.headerSet[strings.ToLower(k)]; hit {
			out[k] = Mask
			continue
		}
		out[k] = v
	}
	return out
}

// Body redacts a raw body according to its content type. JSON bodies get
// per-field masking; everything else falls back to regex pattern scrubbing.
// The returned slice is always safe to retain (independent allocation when
// any change happened).
func (r *Redactor) Body(body []byte, contentType string) []byte {
	if len(body) == 0 {
		return body
	}
	if isJSON(contentType, body) {
		if scrubbed, ok := r.maskJSON(body); ok {
			return r.maskPatterns(scrubbed)
		}
	}
	return r.maskPatterns(body)
}

// maskJSON walks an arbitrarily-nested JSON document and masks fields whose
// keys appear in the denylist. Returns (output, true) on success; (input, false)
// when the body isn't valid JSON (caller falls back to pattern scrubbing).
func (r *Redactor) maskJSON(body []byte) ([]byte, bool) {
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return body, false
	}
	r.maskJSONValue(v)
	out, err := json.Marshal(v)
	if err != nil {
		return body, false
	}
	return out, true
}

func (r *Redactor) maskJSONValue(v any) {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			if _, hit := r.jsonSet[strings.ToLower(k)]; hit {
				t[k] = Mask
				continue
			}
			r.maskJSONValue(child)
		}
	case []any:
		for _, child := range t {
			r.maskJSONValue(child)
		}
	}
}

// maskPatterns scans for regex secrets and replaces matches with Mask.
// Credit-card matches additionally pass a Luhn check to suppress false positives.
func (r *Redactor) maskPatterns(body []byte) []byte {
	if len(r.patterns) == 0 || len(body) == 0 {
		return body
	}
	out := body
	for _, p := range r.patterns {
		if p.Name == "credit_card" {
			out = p.Re.ReplaceAllFunc(out, func(m []byte) []byte {
				if luhn(m) {
					return []byte(Mask)
				}
				return m
			})
			continue
		}
		out = p.Re.ReplaceAll(out, []byte(Mask))
	}
	return out
}

func isJSON(contentType string, body []byte) bool {
	if strings.Contains(strings.ToLower(contentType), "json") {
		return true
	}
	trim := bytes.TrimSpace(body)
	return len(trim) > 0 && (trim[0] == '{' || trim[0] == '[')
}

// luhn validates a sequence of digits (ignoring spaces and hyphens). Returns
// true when the digits form a Luhn-valid number, which strongly correlates
// with credit card numbers and reduces false-positive masking.
func luhn(b []byte) bool {
	var sum, n int
	odd := true
	for i := len(b) - 1; i >= 0; i-- {
		c := b[i]
		if c == ' ' || c == '-' {
			continue
		}
		if c < '0' || c > '9' {
			return false
		}
		d := int(c - '0')
		if !odd {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		n++
		odd = !odd
	}
	return n >= 13 && n <= 19 && sum%10 == 0
}
