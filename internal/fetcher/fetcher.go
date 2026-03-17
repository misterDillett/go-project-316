package fetcher

import (
    "context"
    "fmt"
    "io"
    "net/http"
    "time"

    "code/internal/ratelimiter"
)

type Fetcher struct {
    client    interface {
        Do(req *http.Request) (*http.Response, error)
    }
    retries   int
    limiter   *ratelimiter.RateLimiter
    userAgent string
}

func New(client interface {
    Do(req *http.Request) (*http.Response, error)
}, retries int, limiter *ratelimiter.RateLimiter, userAgent string) *Fetcher {
    return &Fetcher{
        client:    client,
        retries:   retries,
        limiter:   limiter,
        userAgent: userAgent,
    }
}

func (f *Fetcher) Fetch(ctx context.Context, url string) (int, []byte, error) {
    for attempt := 0; attempt <= f.retries; attempt++ {
        if attempt > 0 {
            select {
            case <-time.After(time.Duration(attempt) * time.Second):
            case <-ctx.Done():
                return 0, nil, ctx.Err()
            }
        }

        if f.limiter != nil {
            if !f.limiter.Wait(ctx) {
                return 0, nil, ctx.Err()
            }
        }

        req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
        if err != nil {
            if attempt == f.retries {
                return 0, nil, err
            }
            continue
        }

        if f.userAgent != "" {
            req.Header.Set("User-Agent", f.userAgent)
        }

        resp, err := f.client.Do(req)
        if err != nil {
            if attempt == f.retries {
                return 0, nil, err
            }
            continue
        }

        body, err := io.ReadAll(resp.Body)
        _ = resp.Body.Close()

        if err != nil {
            if attempt == f.retries {
                return resp.StatusCode, nil, err
            }
            continue
        }

        if resp.StatusCode >= 200 && resp.StatusCode < 300 {
            return resp.StatusCode, body, nil
        }

        if !isRetryableStatusCode(resp.StatusCode) {
            return resp.StatusCode, body, nil
        }
    }

    return 0, nil, fmt.Errorf("max retries exceeded")
}

func (f *Fetcher) FetchHead(ctx context.Context, url string) (int, error) {
    for attempt := 0; attempt <= f.retries; attempt++ {
        if attempt > 0 {
            select {
            case <-time.After(time.Duration(attempt) * time.Second):
            case <-ctx.Done():
                return 0, ctx.Err()
            }
        }

        if f.limiter != nil {
            if !f.limiter.Wait(ctx) {
                return 0, ctx.Err()
            }
        }

        req, err := http.NewRequestWithContext(ctx, "HEAD", url, nil)
        if err != nil {
            if attempt == f.retries {
                return 0, err
            }
            continue
        }

        if f.userAgent != "" {
            req.Header.Set("User-Agent", f.userAgent)
        }

        resp, err := f.client.Do(req)
        if err != nil {
            if attempt == f.retries {
                return 0, err
            }
            continue
        }

        statusCode := resp.StatusCode
        _ = resp.Body.Close()

        if statusCode == 405 {
            return f.FetchHeadWithGet(ctx, url)
        }

        if statusCode < 500 && statusCode != 408 && statusCode != 429 {
            return statusCode, nil
        }

        if !isRetryableStatusCode(statusCode) {
            return statusCode, nil
        }
    }

    return 0, fmt.Errorf("max retries exceeded")
}

func (f *Fetcher) FetchHeadWithGet(ctx context.Context, url string) (int, error) {
    for attempt := 0; attempt <= f.retries; attempt++ {
        if attempt > 0 {
            select {
            case <-time.After(time.Duration(attempt) * time.Second):
            case <-ctx.Done():
                return 0, ctx.Err()
            }
        }

        if f.limiter != nil {
            if !f.limiter.Wait(ctx) {
                return 0, ctx.Err()
            }
        }

        req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
        if err != nil {
            if attempt == f.retries {
                return 0, err
            }
            continue
        }

        if f.userAgent != "" {
            req.Header.Set("User-Agent", f.userAgent)
        }

        resp, err := f.client.Do(req)
        if err != nil {
            if attempt == f.retries {
                return 0, err
            }
            continue
        }

        statusCode := resp.StatusCode
        _, _ = io.CopyN(io.Discard, resp.Body, 1024)
        _ = resp.Body.Close()

        if statusCode < 500 && statusCode != 408 && statusCode != 429 {
            if statusCode == 404 {
                return statusCode, fmt.Errorf("not found")
            }
            return statusCode, nil
        }
    }

    return 0, fmt.Errorf("max retries exceeded")
}

func isRetryableStatusCode(statusCode int) bool {
    return statusCode == 408 || statusCode == 429 ||
           statusCode == 500 || statusCode == 502 ||
           statusCode == 503 || statusCode == 504
}