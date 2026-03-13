package crawler

import (
    "net/http"
    "time"
)

func DefaultOptions() Options {
    return Options{
        Depth:       10,
        Retries:     1,
        Delay:       0,
        Timeout:     15 * time.Second,
        UserAgent:   "HexletCrawler/1.0",
        Concurrency: 4,
        IndentJSON:  false,
    }
}

func NewHTTPClient(timeout time.Duration) *http.Client {
    return &http.Client{
        Timeout: timeout,
    }
}