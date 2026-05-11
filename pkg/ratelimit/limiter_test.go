package ratelimit

import (
	"context"
	"testing"
)

func TestDisabled_AlwaysAllows(t *testing.T) {
	l := Disabled()
	for i := 0; i < 1000; i++ {
		d := l.Allow(context.Background(), 1, 5)
		if !d.Allowed {
			t.Fatalf("disabled limiter must always allow")
		}
	}
}

func TestAllow_ZeroRPSDisables(t *testing.T) {
	l := Disabled()
	d := l.Allow(context.Background(), 1, 0)
	if !d.Allowed {
		t.Fatal("limit=0 must allow")
	}
}
