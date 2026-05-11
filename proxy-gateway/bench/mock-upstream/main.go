// Mock upstream that responds with realistic JSON shaped to the request path.
// Used by the bench harness so traffic looks like a real REST API.
package main

import (
	"encoding/json"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":9000"
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	// Legacy /echo kept for the existing k6.js SLO gate.
	mux.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(time.Millisecond)
		w.Header().Set("Content-Type", r.Header.Get("Content-Type"))
		w.WriteHeader(http.StatusOK)
		_, _ = io.Copy(w, r.Body)
	})
	mux.HandleFunc("/", route)

	_ = http.ListenAndServe(addr, mux)
}

// route dispatches on the first path segment to a handler that returns
// realistic JSON. Unknown paths get a 404 with a JSON error body so the
// proxy still has a real response/body to capture.
func route(w http.ResponseWriter, r *http.Request) {
	// Drain the request body — the proxy should have captured it via TeeReader
	// before we get here. Doing this matches how a real backend behaves.
	_, _ = io.Copy(io.Discard, r.Body)

	// Slight random latency so requests don't all complete at the same instant.
	time.Sleep(time.Duration(500+rand.Intn(2500)) * time.Microsecond)

	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	switch {
	case len(parts) >= 2 && parts[0] == "api" && parts[1] == "users":
		users(w, r, parts)
	case len(parts) >= 2 && parts[0] == "api" && parts[1] == "orders":
		orders(w, r, parts)
	case len(parts) >= 2 && parts[0] == "api" && parts[1] == "products":
		products(w, r, parts)
	case len(parts) >= 2 && parts[0] == "api" && parts[1] == "events":
		events(w, r)
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{
			"error": "not_found", "path": r.URL.Path,
		})
	}
}

func users(w http.ResponseWriter, r *http.Request, parts []string) {
	if r.Method == http.MethodPost {
		writeJSON(w, http.StatusCreated, map[string]any{
			"id":         randID(),
			"email":      "user" + randID() + "@example.com",
			"created_at": time.Now().UTC().Format(time.RFC3339),
		})
		return
	}
	if len(parts) >= 3 {
		writeJSON(w, http.StatusOK, map[string]any{
			"id":     parts[2],
			"email":  "user" + parts[2] + "@example.com",
			"name":   "User " + parts[2],
			"plan":   pick("free", "pro", "team", "enterprise"),
			"active": true,
		})
		return
	}
	list := make([]map[string]any, 20)
	for i := range list {
		list[i] = map[string]any{
			"id":    randID(),
			"email": "user" + randID() + "@example.com",
			"name":  "User " + randID(),
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": list, "page": 1, "total": 1247})
}

func orders(w http.ResponseWriter, r *http.Request, parts []string) {
	if r.Method == http.MethodPost {
		writeJSON(w, http.StatusCreated, map[string]any{
			"id":         "ord_" + randID(),
			"status":     "pending",
			"total":      99.99,
			"currency":   "USD",
			"created_at": time.Now().UTC().Format(time.RFC3339),
		})
		return
	}
	if len(parts) >= 3 {
		writeJSON(w, http.StatusOK, map[string]any{
			"id":       "ord_" + parts[2],
			"status":   pick("pending", "paid", "shipped", "delivered"),
			"total":    49.5 + rand.Float64()*200,
			"currency": "USD",
			"items": []map[string]any{
				{"sku": "SKU-" + randID(), "qty": 1 + rand.Intn(3), "price": 19.99},
				{"sku": "SKU-" + randID(), "qty": 1, "price": 29.5},
			},
			"shipping_address": map[string]any{
				"line1": "123 Main St", "city": "San Francisco", "country": "US", "postal": "94103",
			},
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"orders": []any{}, "page": 1, "total": 0})
}

func products(w http.ResponseWriter, r *http.Request, parts []string) {
	if len(parts) >= 3 {
		writeJSON(w, http.StatusOK, map[string]any{
			"id":          parts[2],
			"name":        "Product " + parts[2],
			"description": "A high-quality product designed for everyday use. Lorem ipsum dolor sit amet.",
			"price":       19.99 + rand.Float64()*80,
			"in_stock":    rand.Intn(2) == 1,
			"tags":        []string{"new", "popular", "sale"},
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"products": []any{}, "page": 1})
}

func events(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusAccepted, map[string]any{"accepted": true, "event_id": randID()})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Request-Id", randID())
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

var alphabet = []byte("abcdefghijklmnopqrstuvwxyz0123456789")

func randID() string {
	b := make([]byte, 12)
	for i := range b {
		b[i] = alphabet[rand.Intn(len(alphabet))]
	}
	return string(b)
}

func pick(opts ...string) string {
	return opts[rand.Intn(len(opts))]
}

