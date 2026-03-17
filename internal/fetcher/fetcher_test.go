package fetcher

import (
    "context"
    "io"
    "net/http"
    "net/url"
    "strings"
    "testing"
    "time"

    "code/internal/ratelimiter"
    "code/internal/testutil"
)

func TestFetcher_Fetch(t *testing.T) {
    mockClient := testutil.NewMockHTTPClient()
    limiter := ratelimiter.New(0, 0)
    if limiter != nil {
        defer limiter.Stop()
    }

    f := New(mockClient, 1, limiter, "TestAgent")

    mockClient.Responses["https://example.com"] = &http.Response{
        StatusCode: 200,
        Body:       io.NopCloser(strings.NewReader("test body")),
    }

    statusCode, body, err := f.Fetch(context.Background(), "https://example.com")
    if err != nil {
        t.Fatalf("Expected no error, got %v", err)
    }
    if statusCode != 200 {
        t.Errorf("Expected status 200, got %d", statusCode)
    }
    if string(body) != "test body" {
        t.Errorf("Expected body 'test body', got '%s'", string(body))
    }
}

func TestFetcher_FetchWithRetry(t *testing.T) {
    mockClient := testutil.NewMockHTTPClient()
    limiter := ratelimiter.New(0, 0)
    if limiter != nil {
        defer limiter.Stop()
    }

    mockClient.Responses["https://example.com"] = &http.Response{
        StatusCode: 500,
        Body:       io.NopCloser(strings.NewReader("error")),
    }

    mockClient.Responses["https://example.com_2"] = &http.Response{
        StatusCode: 200,
        Body:       io.NopCloser(strings.NewReader("success")),
    }

    requestCount := 0
    mockClient.Hook = func(req *http.Request) {
        requestCount++
        if requestCount == 2 {
            req.URL, _ = url.Parse("https://example.com_2")
        }
    }

    f := New(mockClient, 1, limiter, "TestAgent")

    statusCode, body, err := f.Fetch(context.Background(), "https://example.com")
    if err != nil {
        t.Fatalf("Expected no error, got %v", err)
    }
    if statusCode != 200 {
        t.Errorf("Expected status 200, got %d", statusCode)
    }
    if string(body) != "success" {
        t.Errorf("Expected body 'success', got '%s'", string(body))
    }
}

func TestFetcher_FetchHead(t *testing.T) {
    mockClient := testutil.NewMockHTTPClient()
    limiter := ratelimiter.New(0, 0)
    if limiter != nil {
        defer limiter.Stop()
    }

    mockClient.Responses["https://example.com"] = &http.Response{
        StatusCode: 200,
        Header:     http.Header{"Content-Length": []string{"1024"}},
        Body:       io.NopCloser(strings.NewReader("")),
    }

    f := New(mockClient, 1, limiter, "TestAgent")

    statusCode, err := f.FetchHead(context.Background(), "https://example.com")
    if err != nil {
        t.Fatalf("Expected no error, got %v", err)
    }
    if statusCode != 200 {
        t.Errorf("Expected status 200, got %d", statusCode)
    }
}

func TestFetcher_FetchHeadNotFound(t *testing.T) {
    mockClient := testutil.NewMockHTTPClient()
    limiter := ratelimiter.New(0, 0)
    if limiter != nil {
        defer limiter.Stop()
    }

    mockClient.Responses["https://example.com"] = &http.Response{
        StatusCode: 404,
        Body:       io.NopCloser(strings.NewReader("")),
    }

    f := New(mockClient, 1, limiter, "TestAgent")

    statusCode, err := f.FetchHead(context.Background(), "https://example.com")
    if err != nil {
        t.Fatalf("Expected no error, got %v", err)
    }
    if statusCode != 404 {
        t.Errorf("Expected status 404, got %d", statusCode)
    }
}

func TestFetcher_WithRateLimiter(t *testing.T) {
    mockClient := testutil.NewMockHTTPClient()
    limiter := ratelimiter.New(10, 0)
    if limiter != nil {
        defer limiter.Stop()
    }

    mockClient.Responses["https://example.com"] = &http.Response{
        StatusCode: 200,
        Body:       io.NopCloser(strings.NewReader("test")),
    }

    f := New(mockClient, 0, limiter, "TestAgent")

    start := time.Now()
    _, _, err := f.Fetch(context.Background(), "https://example.com")
    elapsed := time.Since(start)

    if err != nil {
        t.Fatalf("Expected no error, got %v", err)
    }
    if elapsed < 90*time.Millisecond {
        t.Errorf("Expected delay at least 90ms, got %v", elapsed)
    }
}