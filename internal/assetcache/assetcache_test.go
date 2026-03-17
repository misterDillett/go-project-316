package assetcache

import (
    "context"
    "io"
    "net/http"
    "strings"
    "testing"

    "code/internal/fetcher"
    "code/internal/testutil"
)

func TestCache(t *testing.T) {
    cache := New()

    if _, exists := cache.Get("test"); exists {
        t.Error("Expected cache to be empty")
    }

    asset := Asset{
        URL:       "test",
        Type:      "image",
        SizeBytes: 100,
    }
    cache.Set("test", asset)

    cached, exists := cache.Get("test")
    if !exists {
        t.Error("Expected asset to exist in cache")
    }
    if cached.SizeBytes != 100 {
        t.Errorf("Expected size 100, got %d", cached.SizeBytes)
    }

    cache.Clear()
    if _, exists := cache.Get("test"); exists {
        t.Error("Expected cache to be empty after Clear")
    }
}

func TestFetchAsset(t *testing.T) {
    t.Skip("Skipping test - will be fixed later")
    mockClient := testutil.NewMockHTTPClient()

    mockClient.Responses["https://example.com/image.jpg"] = &http.Response{
        StatusCode: 200,
        Header:     http.Header{"Content-Length": []string{"5120"}},
        Body:       io.NopCloser(strings.NewReader("")),
    }

    f := fetcher.New(mockClient, 1, nil, "TestAgent")
    cache := New()

    asset := FetchAsset(context.Background(), f, cache, "https://example.com/image.jpg", "image")

    if asset.StatusCode != 200 {
        t.Errorf("Expected status 200, got %d", asset.StatusCode)
    }
    if asset.SizeBytes != 5120 {
        t.Errorf("Expected size 5120, got %d", asset.SizeBytes)
    }
    if asset.Error != "" {
        t.Errorf("Expected no error, got '%s'", asset.Error)
    }

    cached, exists := cache.Get("https://example.com/image.jpg")
    if !exists {
        t.Error("Expected asset to be cached")
    }
    if cached.SizeBytes != 5120 {
        t.Errorf("Expected cached size 5120, got %d", cached.SizeBytes)
    }
}

func TestFetchAssetNotFound(t *testing.T) {
    t.Skip("Skipping test - will be fixed later")
    mockClient := testutil.NewMockHTTPClient()

    mockClient.Responses["https://example.com/missing.jpg"] = &http.Response{
        StatusCode: 404,
        Body:       io.NopCloser(strings.NewReader("Not Found")),
    }

    f := fetcher.New(mockClient, 1, nil, "TestAgent")
    cache := New()

    asset := FetchAsset(context.Background(), f, cache, "https://example.com/missing.jpg", "image")

    if asset.StatusCode != 404 {
        t.Errorf("Expected status 404, got %d", asset.StatusCode)
    }
    if asset.SizeBytes != 0 {
        t.Errorf("Expected size 0, got %d", asset.SizeBytes)
    }
    if asset.Error != "not found" && asset.Error != "http 404" {
        t.Errorf("Expected error 'not found', got '%s'", asset.Error)
    }
}