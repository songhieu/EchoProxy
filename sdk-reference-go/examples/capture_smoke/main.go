// Smoke example for the Go SDK *capture mode*. Wraps http.DefaultTransport
// with capture.Transport — the app makes the upstream call itself and the
// SDK ships the event to ingest-api with source = sdk-go. The orchestrator
// script runs this with a unique tag in the path and verifies the event
// landed in ClickHouse.
package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	sdk "github.com/songhieu/EchoProxy/sdk-reference-go"
	"github.com/songhieu/EchoProxy/sdk-reference-go/capture"
)

func main() {
	tag := os.Getenv("ECHOPROXY_TAG")
	if tag == "" {
		tag = "go-default"
	}
	target := os.Getenv("ECHOPROXY_EXAMPLE_TARGET")
	if target == "" {
		target = "http://upstream-mock:9000"
	}
	endpoint := os.Getenv("ECHOPROXY_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://localhost:8081"
	}
	apiKey := os.Getenv("ECHOPROXY_API_KEY")
	if apiKey == "" {
		apiKey = "sk_test_demo"
	}

	client, err := sdk.New(sdk.Config{
		APIKey:        apiKey,
		EndpointHTTP:  endpoint,
		FlushInterval: 200 * time.Millisecond,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "sdk.New: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = client.Close(ctx)
	}()

	httpClient := &http.Client{Transport: capture.NewTransport(nil, client)}

	url := fmt.Sprintf("%s/api/users/sdkbench-go-capture-%s", target, tag)
	res, err := httpClient.Get(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "go sdk (capture) error: %v\n", err)
		os.Exit(1)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)

	fmt.Printf("go sdk (capture): %s -> %d (%d bytes)\n", url, res.StatusCode, len(body))
	if res.StatusCode != 200 {
		os.Exit(1)
	}
}
