package main

import "testing"

func TestResolveAddressRequiresLoopback(t *testing.T) {
	if _, err := resolveAddress("0.0.0.0:19081"); err == nil {
		t.Fatal("expected wildcard address rejection")
	}
	address, err := resolveAddress("127.0.0.1:19999")
	if err != nil || address != "127.0.0.1:19999" {
		t.Fatalf("unexpected address result: %q, %v", address, err)
	}
}
