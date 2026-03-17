package ratelimiter

import (
    "context"
    "time"
)

type RateLimiter struct {
    ticker *time.Ticker
    done   chan bool
}

func New(rps int, delay time.Duration) *RateLimiter {
    var interval time.Duration

    if rps > 0 {
        interval = time.Second / time.Duration(rps)
    } else if delay > 0 {
        interval = delay
    } else {
        return nil
    }

    return &RateLimiter{
        ticker: time.NewTicker(interval),
        done:   make(chan bool),
    }
}

func (r *RateLimiter) Wait(ctx context.Context) bool {
    if r == nil {
        return true
    }

    select {
    case <-r.ticker.C:
        return true
    case <-ctx.Done():
        return false
    case <-r.done:
        return false
    }
}

func (r *RateLimiter) Stop() {
    if r != nil {
        r.ticker.Stop()
        close(r.done)
    }
}