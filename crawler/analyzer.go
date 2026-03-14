package crawler

import (
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "net/url"
    "strings"
    "time"
    "sync"
)

type RateLimiter struct {
    ticker *time.Ticker
    done   chan bool
}

var (
    assetCache = make(map[string]Asset)
    assetMu    sync.Mutex
)

func NewRateLimiter(rps int, delay time.Duration) *RateLimiter {
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

    limiter := NewRateLimiter(0, opts.Delay)
    if limiter != nil {
        defer limiter.Stop()
    }

    assetCache = make(map[string]Asset)

    visited := make(map[string]bool)
    var allPages []Page

    type queueItem struct {
        url   string
        depth int
    }
    queue := []queueItem{{url: opts.URL, depth: 0}}

    for i := 0; i < len(queue); i++ {
        select {
        case <-ctx.Done():
            report.Pages = allPages
            return json.MarshalIndent(report, "", "  ")
        default:
        }

        item := queue[i]

        if item.depth > opts.Depth {
            continue
        }

        if visited[item.url] {
            continue
        }
        visited[item.url] = true

        page, internalLinks := fetchPageWithInternal(ctx, opts, limiter, item.url, item.depth)
        allPages = append(allPages, page)

        for _, link := range internalLinks {
            if !visited[link] {
                queue = append(queue, queueItem{url: link, depth: item.depth + 1})
            }
        }

        if opts.Delay > 0 {
            time.Sleep(opts.Delay)
        }
    }

    report.Pages = allPages

    var jsonData []byte
    var err error
    if opts.IndentJSON {
        jsonData, err = json.MarshalIndent(report, "", "  ")
    } else {
        jsonData, err = json.Marshal(report)
    }

    if err != nil {
        return nil, fmt.Errorf("failed to marshal report: %w", err)
    }

    return jsonData, nil
}

func fetchPageWithInternal(ctx context.Context, opts Options, limiter *RateLimiter, pageURL string, depth int) (Page, []string) {
    page := Page{
        URL:          pageURL,
        Depth:        depth,
        DiscoveredAt: time.Now().UTC(),
        SEO:          SEO{},
        BrokenLinks:  []BrokenLink{},
        Assets:       []Asset{},
    }

    var internalLinks []string
    var lastErr error
    var lastStatusCode int

    for attempt := 0; attempt <= opts.Retries; attempt++ {
        if attempt > 0 {
            select {
            case <-time.After(time.Duration(attempt) * time.Second):
            case <-ctx.Done():
                page.HTTPStatus = 0
                page.Status = "error"
                page.Error = ctx.Err().Error()
                return page, internalLinks
            }
        }

        if limiter != nil {
            if !limiter.Wait(ctx) {
                page.HTTPStatus = 0
                page.Status = "error"
                page.Error = "cancelled by rate limiter"
                return page, internalLinks
            }
        }

        req, err := http.NewRequestWithContext(ctx, "GET", pageURL, nil)
        if err != nil {
            lastErr = err
            continue
        }

        if opts.UserAgent != "" {
            req.Header.Set("User-Agent", opts.UserAgent)
        }

        resp, err := opts.HTTPClient.Do(req)
        if err != nil {
            lastErr = err
            continue
        }

        body, err := io.ReadAll(resp.Body)
        lastStatusCode = resp.StatusCode
        resp.Body.Close()

        if err != nil {
            lastErr = err
            continue
        }

        if resp.StatusCode >= 200 && resp.StatusCode < 300 {
            page.HTTPStatus = resp.StatusCode
            page.Status = "ok"
            page.Error = ""

            if isHTMLContent(resp.Header.Get("Content-Type")) {
                seoTags := ParseSEOTags(body)
                page.SEO = SEO{
                    HasTitle:       seoTags.Title != "",
                    Title:          seoTags.Title,
                    HasDescription: seoTags.Description != "",
                    Description:    seoTags.Description,
                    HasH1:          seoTags.H1 != "",
                    H1:             seoTags.H1,
                }

                links, err := ParseLinks(pageURL, body)
                if err == nil {
                    for _, link := range links {
                        absURL := resolveURL(pageURL, link.URL)
                        statusCode, err := checkLink(ctx, opts, limiter, absURL)

                        if err != nil || statusCode >= 400 {
                            brokenLink := BrokenLink{
                                URL: absURL,
                            }
                            if err != nil {
                                brokenLink.Error = err.Error()
                            } else {
                                brokenLink.StatusCode = statusCode
                                if statusCode == 404 {
                                    brokenLink.Error = "Not Found"
                                }
                            }
                            page.BrokenLinks = append(page.BrokenLinks, brokenLink)
                        }

                        if err == nil && statusCode >= 200 && statusCode < 300 &&
                           isSameDomain(absURL, opts.URL) && depth < opts.Depth {
                            if !strings.Contains(absURL, ".jpg") &&
                               !strings.Contains(absURL, ".png") &&
                               !strings.Contains(absURL, ".css") &&
                               !strings.Contains(absURL, ".js") &&
                               !strings.Contains(absURL, ".svg") {
                                internalLinks = append(internalLinks, absURL)
                            }
                        }
                    }
                }

                assets := ParseAssets(pageURL, body)
                seen := make(map[string]bool)
                for _, asset := range assets {
                    if !seen[asset.URL] {
                        seen[asset.URL] = true
                        assetInfo := fetchAsset(ctx, opts, limiter, asset.URL, asset.Type)
                        page.Assets = append(page.Assets, assetInfo)
                    }
                }
            }
            return page, internalLinks
        }

        if !isRetryableStatusCode(resp.StatusCode) {
            page.HTTPStatus = resp.StatusCode
            page.Status = "error"
            page.Error = http.StatusText(resp.StatusCode)
            return page, internalLinks
        }

        lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
    }

    page.HTTPStatus = lastStatusCode
    page.Status = "error"
    if lastErr != nil {
        page.Error = lastErr.Error()
    } else {
        page.Error = "max retries exceeded"
    }
    return page, internalLinks
}

func fetchAsset(ctx context.Context, opts Options, limiter *RateLimiter, assetURL, assetType string) Asset {
    assetMu.Lock()
    if cached, exists := assetCache[assetURL]; exists {
        assetMu.Unlock()
        return cached
    }
    assetMu.Unlock()

    asset := Asset{
        URL:  assetURL,
        Type: assetType,
    }

    for attempt := 0; attempt <= opts.Retries; attempt++ {
        if attempt > 0 {
            select {
            case <-time.After(time.Duration(attempt) * time.Second):
            case <-ctx.Done():
                asset.Error = ctx.Err().Error()
                return asset
            }
        }

        if limiter != nil {
            if !limiter.Wait(ctx) {
                asset.Error = "cancelled by rate limiter"
                return asset
            }
        }

        req, err := http.NewRequestWithContext(ctx, "HEAD", assetURL, nil)
        if err != nil {
            continue
        }

        if opts.UserAgent != "" {
            req.Header.Set("User-Agent", opts.UserAgent)
        }

        resp, err := opts.HTTPClient.Do(req)
        if err != nil {
            continue
        }

        asset.StatusCode = resp.StatusCode

        if resp.StatusCode >= 400 {
            asset.SizeBytes = 0
            if resp.StatusCode == 404 {
                asset.Error = "Not Found"
            } else {
                asset.Error = http.StatusText(resp.StatusCode)
            }
            resp.Body.Close()

            // Сохраняем в кэш
            assetMu.Lock()
            assetCache[assetURL] = asset
            assetMu.Unlock()
            return asset
        }

        if resp.ContentLength > 0 {
            asset.SizeBytes = resp.ContentLength
            resp.Body.Close()

            if resp.StatusCode >= 200 && resp.StatusCode < 300 {
                asset.Error = ""

                assetMu.Lock()
                assetCache[assetURL] = asset
                assetMu.Unlock()
                return asset
            }
        } else {
            resp.Body.Close()
        }

        if limiter != nil {
            if !limiter.Wait(ctx) {
                asset.Error = "cancelled by rate limiter"
                return asset
            }
        }

        getReq, err := http.NewRequestWithContext(ctx, "GET", assetURL, nil)
        if err != nil {
            continue
        }

        if opts.UserAgent != "" {
            getReq.Header.Set("User-Agent", opts.UserAgent)
        }

        getResp, err := opts.HTTPClient.Do(getReq)
        if err != nil {
            continue
        }

        asset.StatusCode = getResp.StatusCode

        if getResp.StatusCode >= 400 {
            asset.SizeBytes = 0
            if getResp.StatusCode == 404 {
                asset.Error = "Not Found"
            } else {
                asset.Error = http.StatusText(getResp.StatusCode)
            }
            getResp.Body.Close()

            assetMu.Lock()
            assetCache[assetURL] = asset
            assetMu.Unlock()
            return asset
        }

        asset.SizeBytes, _ = io.Copy(io.Discard, getResp.Body)
        getResp.Body.Close()

        if getResp.StatusCode >= 200 && getResp.StatusCode < 300 {
            asset.Error = ""

            assetMu.Lock()
            assetCache[assetURL] = asset
            assetMu.Unlock()
            return asset
        }
    }

    asset.Error = "max retries exceeded"
    asset.SizeBytes = 0
    return asset
}

func isRetryableStatusCode(statusCode int) bool {
    return statusCode == 408 || statusCode == 429 ||
           statusCode == 500 || statusCode == 502 ||
           statusCode == 503 || statusCode == 504
}

func isHTMLContent(contentType string) bool {
    return contentType == "" ||
        contentType == "text/html" ||
        strings.Contains(contentType, "text/html")
}

func checkLink(ctx context.Context, opts Options, limiter *RateLimiter, linkURL string) (int, error) {
    for attempt := 0; attempt <= opts.Retries; attempt++ {
        if attempt > 0 {
            select {
            case <-time.After(time.Duration(attempt) * time.Second):
            case <-ctx.Done():
                return 0, ctx.Err()
            }
        }

        if limiter != nil {
            if !limiter.Wait(ctx) {
                return 0, ctx.Err()
            }
        }

        req, err := http.NewRequestWithContext(ctx, "HEAD", linkURL, nil)
        if err != nil {
            continue
        }

        if opts.UserAgent != "" {
            req.Header.Set("User-Agent", opts.UserAgent)
        }

        resp, err := opts.HTTPClient.Do(req)
        if err != nil {
            continue
        }

        if resp.StatusCode == 405 {
            resp.Body.Close()
            return checkLinkWithGET(ctx, opts, limiter, linkURL)
        }

        resp.Body.Close()

        if resp.StatusCode < 500 && resp.StatusCode != 408 && resp.StatusCode != 429 {
            return resp.StatusCode, nil
        }
    }

    return 0, fmt.Errorf("max retries exceeded")
}

func checkLinkWithGET(ctx context.Context, opts Options, limiter *RateLimiter, linkURL string) (int, error) {
    for attempt := 0; attempt <= opts.Retries; attempt++ {
        if attempt > 0 {
            select {
            case <-time.After(time.Duration(attempt) * time.Second):
            case <-ctx.Done():
                return 0, ctx.Err()
            }
        }

        if limiter != nil {
            if !limiter.Wait(ctx) {
                return 0, ctx.Err()
            }
        }

        req, err := http.NewRequestWithContext(ctx, "GET", linkURL, nil)
        if err != nil {
            continue
        }

        if opts.UserAgent != "" {
            req.Header.Set("User-Agent", opts.UserAgent)
        }

        resp, err := opts.HTTPClient.Do(req)
        if err != nil {
            continue
        }

        _, err = io.CopyN(io.Discard, resp.Body, 1024)
        resp.Body.Close()

        if resp.StatusCode < 500 && resp.StatusCode != 408 && resp.StatusCode != 429 {
            if resp.StatusCode == 404 {
                return resp.StatusCode, fmt.Errorf("Not Found")
            }
            return resp.StatusCode, nil
        }
    }

    return 0, fmt.Errorf("max retries exceeded")
}

func resolveURL(base, ref string) string {
    baseURL, err := url.Parse(base)
    if err != nil {
        return ref
    }

    refURL, err := url.Parse(ref)
    if err != nil {
        return ref
    }

    return baseURL.ResolveReference(refURL).String()
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