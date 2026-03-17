package assetcache

import (
    "context"
    "fmt"
    "io"
    "net/http"

    "code/internal/fetcher"
)

type Asset struct {
    URL        string
    Type       string
    StatusCode int
    SizeBytes  int64
    Error      string
}

type Cache struct {
    assets map[string]Asset
}

func New() *Cache {
    return &Cache{
        assets: make(map[string]Asset),
    }
}

func (c *Cache) Clear() {
    c.assets = make(map[string]Asset)
}

func (c *Cache) Get(url string) (Asset, bool) {
    asset, exists := c.assets[url]
    return asset, exists
}

func (c *Cache) Set(url string, asset Asset) {
    c.assets[url] = asset
}

func FetchAsset(ctx context.Context, f *fetcher.Fetcher, cache *Cache, assetURL, assetType string) Asset {
    if cached, exists := cache.Get(assetURL); exists {
        return cached
    }

    asset := Asset{
        URL:  assetURL,
        Type: assetType,
    }

    statusCode, size, err := fetchAssetHead(ctx, f, assetURL)
    if err == nil && statusCode >= 200 && statusCode < 300 {
        asset.StatusCode = statusCode
        asset.SizeBytes = size
        asset.Error = ""
        cache.Set(assetURL, asset)
        return asset
    }

    statusCode, size, err = fetchAssetGet(ctx, f, assetURL)
    if err == nil && statusCode >= 200 && statusCode < 300 {
        asset.StatusCode = statusCode
        asset.SizeBytes = size
        asset.Error = ""
        cache.Set(assetURL, asset)
        return asset
    }

    if err != nil {
        asset.Error = err.Error()
    } else {
        asset.StatusCode = statusCode
        if statusCode == 404 {
            asset.Error = "not found"
        } else {
            asset.Error = http.StatusText(statusCode)
        }
    }
    asset.SizeBytes = 0
    cache.Set(assetURL, asset)
    return asset
}

func fetchAssetHead(ctx context.Context, f *fetcher.Fetcher, assetURL string) (int, int64, error) {
    req, err := http.NewRequestWithContext(ctx, "HEAD", assetURL, nil)
    if err != nil {
        return 0, 0, err
    }

    client := &http.Client{}
    resp, err := client.Do(req)
    if err != nil {
        return 0, 0, err
    }
    defer resp.Body.Close()

    if resp.StatusCode >= 400 {
        return resp.StatusCode, 0, fmt.Errorf("http %d", resp.StatusCode)
    }

    return resp.StatusCode, resp.ContentLength, nil
}

func fetchAssetGet(ctx context.Context, f *fetcher.Fetcher, assetURL string) (int, int64, error) {
    req, err := http.NewRequestWithContext(ctx, "GET", assetURL, nil)
    if err != nil {
        return 0, 0, err
    }

    client := &http.Client{}
    resp, err := client.Do(req)
    if err != nil {
        return 0, 0, err
    }
    defer resp.Body.Close()

    if resp.StatusCode >= 400 {
        return resp.StatusCode, 0, fmt.Errorf("http %d", resp.StatusCode)
    }

    size, err := io.Copy(io.Discard, resp.Body)
    if err != nil {
        return resp.StatusCode, 0, err
    }

    return resp.StatusCode, size, nil
}