package main

import (
	"sync"
	"testing"
	"time"
)

func TestWaitForRelayReturnsWhenWorkerStops(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
	}()
	if !waitForRelay(&wg, time.Second) {
		t.Fatal("relay wait timed out after the worker stopped")
	}
}

func TestWaitForRelayHonoursTimeout(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	start := time.Now()
	if waitForRelay(&wg, 20*time.Millisecond) {
		t.Fatal("relay wait returned before the worker stopped")
	}
	if elapsed := time.Since(start); elapsed < 15*time.Millisecond {
		t.Fatalf("relay wait returned too early: %s", elapsed)
	}
	wg.Done()
}
