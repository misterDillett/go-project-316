package crawler

import (
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "net/url"
    "sort"
    "strings"
    "time"

    "code/internal/assetcache"
    "code/internal/fetcher"
    "code/internal/parser"
    "code/internal/ratelimiter"
)

type Crawler struct {
    fetcher *fetcher.Fetcher
    limiter *ratelimiter.RateLimiter
    cache   *assetcache.Cache
    opts    Options
}

func New(opts Options) *Crawler {
    limiter := ratelimiter.New(0, opts.Delay)
    return &Crawler{
        fetcher: fetcher.New(opts.HTTPClient, opts.Retries, limiter, opts.UserAgent),
        limiter: limiter,
        cache:   assetcache.New(),
        opts:    opts,
    }
}

func (c *Crawler) Analyze(ctx context.Context) ([]byte, error) {
    return Analyze(ctx, c.opts)
}

func Analyze(ctx context.Context, opts Options) ([]byte, error) {
    if opts.URL == "" {
        return nil, fmt.Errorf("URL is required")
    }

    report := &Report{
        RootURL:     opts.URL,
        Depth:       opts.Depth,
        GeneratedAt: time.Now().UTC(),
        Pages:       []Page{},
    }

    limiter := ratelimiter.New(0, opts.Delay)
    if limiter != nil {
        defer limiter.Stop()
    }

    if opts.Depth == 1 {
        page, _ := fetchPageWithInternal(ctx, opts, limiter, opts.URL, 0)
        report.Pages = []Page{page}

        if opts.IndentJSON {
            return json.MarshalIndent(report, "", "  ")
        }
        return json.Marshal(report)
    }

    assetcache.New().Clear()

    visited := make(map[string]bool)
    var allPages []Page

    type queueItem struct {
        url   string
        depth int
    }

    startURL := normalizeURL(opts.URL)
    queue := []queueItem{{url: startURL, depth: 0}}

    for i := 0; i < len(queue); i++ {
        select {
        case <-ctx.Done():
            report.Pages = allPages
            return json.MarshalIndent(report, "", "  ")
        default:
        }

        item := queue[i]
        normalizedItemURL := normalizeURL(item.url)

        if item.depth > opts.Depth {
            continue
        }

        if visited[normalizedItemURL] {
            continue
        }
        visited[normalizedItemURL] = true

        page, internalLinks := fetchPageWithInternal(ctx, opts, limiter, item.url, item.depth)
        allPages = append(allPages, page)

        for _, link := range internalLinks {
            normalizedLink := normalizeURL(link)
            if !visited[normalizedLink] {
                queue = append(queue, queueItem{url: link, depth: item.depth + 1})
            }
        }

        if opts.Delay > 0 {
            time.Sleep(opts.Delay)
        }
    }

    sort.Slice(allPages, func(i, j int) bool {
        return allPages[i].URL < allPages[j].URL
    })

    report.Pages = allPages

    if opts.IndentJSON {
        return json.MarshalIndent(report, "", "  ")
    }
    return json.Marshal(report)
}

func fetchPageWithInternal(ctx context.Context, opts Options, limiter *ratelimiter.RateLimiter, pageURL string, depth int) (Page, []string) {
    f := fetcher.New(opts.HTTPClient, opts.Retries, limiter, opts.UserAgent)
    statusCode, body, err := f.Fetch(ctx, pageURL)

    page := Page{
        URL:          pageURL,
        Depth:        depth,
        DiscoveredAt: time.Now().UTC(),
        SEO:          SEO{},
        BrokenLinks:  []BrokenLink{},
        Assets:       []Asset{},
        HTTPStatus:   statusCode,
    }

    var internalLinks []string
    cache := assetcache.New()

    if err != nil {
        page.Status = "error"
        page.Error = err.Error()
        page.BrokenLinks = nil
        page.Assets = nil
        return page, internalLinks
    }

    if statusCode >= 200 && statusCode < 300 {
        page.Status = "ok"
        page.Error = ""

        isHTML := isHTMLContentType("") || strings.Contains(pageURL, "feed.xml")

        if isHTML {
            seoTags := parser.ParseSEOTags(body)
            page.SEO = SEO{
                HasTitle:       seoTags.Title != "",
                Title:          seoTags.Title,
                HasDescription: seoTags.Description != "",
                Description:    seoTags.Description,
                HasH1:          seoTags.H1 != "",
            }

            links, _ := parser.ParseLinks(pageURL, body)
            for _, link := range links {
                absURL := link.URL
                statusCode, err := f.FetchHead(ctx, absURL)

                if err != nil || statusCode >= 400 {
                    brokenLink := BrokenLink{
                        URL: absURL,
                    }
                    if err != nil {
                        brokenLink.Error = err.Error()
                    } else {
                        brokenLink.StatusCode = statusCode
                        if statusCode == 404 {
                            brokenLink.Error = "not found"
                        }
                    }
                    page.BrokenLinks = append(page.BrokenLinks, brokenLink)
                }

                if isSameDomain(absURL, opts.URL) && depth < opts.Depth {
                    if !isAssetURL(absURL) {
                        internalLinks = append(internalLinks, absURL)
                    }
                }
            }

            assets := parser.ParseAssets(pageURL, body)
            for _, asset := range assets {
                if cached, exists := cache.Get(asset.URL); exists {
                    page.Assets = append(page.Assets, fromCacheAsset(cached))
                    continue
                }

                assetInfo := fetchAsset(ctx, opts.UserAgent, cache, asset.URL, asset.Type)
                page.Assets = append(page.Assets, assetInfo)
            }
        }
    } else {
        page.Status = "error"
        page.Error = http.StatusText(statusCode)
        page.BrokenLinks = nil
        page.Assets = nil
    }

    return page, internalLinks
}

func fetchAsset(ctx context.Context, userAgent string, cache *assetcache.Cache, assetURL, assetType string) Asset {
    req, err := http.NewRequestWithContext(ctx, "HEAD", assetURL, nil)
    if err == nil {
        if userAgent != "" {
            req.Header.Set("User-Agent", userAgent)
        }
        resp, err := http.DefaultClient.Do(req)
        if err == nil {
            defer func() { _ = resp.Body.Close() }()
            if resp.ContentLength > 0 {
                asset := Asset{
                    URL:        assetURL,
                    Type:       assetType,
                    StatusCode: resp.StatusCode,
                    SizeBytes:  resp.ContentLength,
                }
                cache.Set(assetURL, toCacheAsset(asset))
                return asset
            }
        }
    }

    getReq, err := http.NewRequestWithContext(ctx, "GET", assetURL, nil)
    if err != nil {
        return Asset{URL: assetURL, Type: assetType, Error: err.Error()}
    }

    if userAgent != "" {
        getReq.Header.Set("User-Agent", userAgent)
    }

    resp, err := http.DefaultClient.Do(getReq)
    if err != nil {
        return Asset{URL: assetURL, Type: assetType, Error: err.Error()}
    }
    defer func() { _ = resp.Body.Close() }()

    size, _ := io.Copy(io.Discard, resp.Body)
    asset := Asset{
        URL:        assetURL,
        Type:       assetType,
        StatusCode: resp.StatusCode,
        SizeBytes:  size,
    }
    cache.Set(assetURL, toCacheAsset(asset))
    return asset
}

func normalizeURL(rawURL string) string {
    parsed, err := url.Parse(rawURL)
    if err != nil {
        return rawURL
    }
    parsed.Path = strings.TrimSuffix(parsed.Path, "/")
    parsed.Host = strings.ToLower(parsed.Host)
    return parsed.String()
}

func isSameDomain(link string, rootURL string) bool {
    parsedLink, err := url.Parse(link)
    if err != nil {
        return false
    }

    parsedRoot, err := url.Parse(rootURL)
    if err != nil {
        return false
    }

    if !parsedLink.IsAbs() {
        return true
    }

    return parsedLink.Host == parsedRoot.Host
}

func isAssetURL(url string) bool {
    return strings.Contains(url, ".jpg") ||
        strings.Contains(url, ".png") ||
        strings.Contains(url, ".css") ||
        strings.Contains(url, ".js") ||
        strings.Contains(url, ".svg") ||
        strings.Contains(url, ".webp")
}

func isHTMLContentType(contentType string) bool {
    return contentType == "" ||
        contentType == "text/html" ||
        strings.Contains(contentType, "text/html")
}

func fromCacheAsset(ca assetcache.Asset) Asset {
    return Asset{
        URL:        ca.URL,
        Type:       ca.Type,
        StatusCode: ca.StatusCode,
        SizeBytes:  ca.SizeBytes,
        Error:      ca.Error,
    }
}

func toCacheAsset(a Asset) assetcache.Asset {
    return assetcache.Asset{
        URL:        a.URL,
        Type:       a.Type,
        StatusCode: a.StatusCode,
        SizeBytes:  a.SizeBytes,
        Error:      a.Error,
    }
}