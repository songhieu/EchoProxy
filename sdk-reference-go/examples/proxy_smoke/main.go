// Smoke example for the Go SDK proxy mode. Routes one request through the
// EchoProxy proxy and prints the response status. The orchestrator script
// (../../bench/run-sdk-smoke.sh) runs this with a unique tag in the path
// and then verifies the event landed in ClickHouse.
package main

import (
	"fmt"
	"io"
	"os"

	sid "github.com/songhieu/EchoProxy/sdk-reference-go/proxy"
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

	url := fmt.Sprintf("%s/api/users/sdkbench-go-%s", target, tag)
	res, err := sid.Get(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "go sdk error: %v\n", err)
		os.Exit(1)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)

	fmt.Printf("go sdk: %s -> %d (%d bytes)\n", url, res.StatusCode, len(body))
	if res.StatusCode != 200 {
		os.Exit(1)
	}
}
