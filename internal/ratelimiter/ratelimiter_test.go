package ratelimiter

import (
    "context"
    "testing"
    "time"
)

func TestNew(t *testing.T) {
    tests := []struct {
        name  string
        rps   int
        delay time.Duration
        want  bool
    }{
        {"No limiter", 0, 0, false},
        {"With RPS", 10, 0, true},
        {"With delay", 0, 100 * time.Millisecond, true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            limiter := New(tt.rps, tt.delay)
            if (limiter != nil) != tt.want {
                t.Errorf("New() = %v, want %v", limiter != nil, tt.want)
            }
            if limiter != nil {
                limiter.Stop()
            }
        })
    }
}

func TestWait(t *testing.T) {
    limiter := New(10, 0)
    if limiter == nil {
        t.Fatal("Expected limiter, got nil")
    }
    defer limiter.Stop()

    start := time.Now()
    ok := limiter.Wait(context.Background())
    elapsed := time.Since(start)

    if !ok {
        t.Error("Expected Wait to return true")
    }
    if elapsed < 90*time.Millisecond {
        t.Errorf("Expected wait at least 90ms, got %v", elapsed)
    }
}

func TestWaitWithCancel(t *testing.T) {
    limiter := New(1, 0)
    if limiter == nil {
        t.Fatal("Expected limiter, got nil")
    }
    defer limiter.Stop()

    ctx, cancel := context.WithCancel(context.Background())
    cancel()

    start := time.Now()
    ok := limiter.Wait(ctx)
    elapsed := time.Since(start)

    if ok {
        t.Error("Expected Wait to return false")
    }
    if elapsed > 10*time.Millisecond {
        t.Errorf("Expected fast return, got %v", elapsed)
    }
}