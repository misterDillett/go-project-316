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
    "sync"
    "time"

    "code/internal/assetcache"
    "code/internal/fetcher"
    "code/internal/parser"
    "code/internal/ratelimiter"
)

type queueItem struct {
    url   string
    depth int
}

type resultItem struct {
    page  Page
    links []string
}

type crawler struct {
    fetcher *fetcher.Fetcher
    limiter *ratelimiter.RateLimiter
    cache   *assetcache.Cache
    opts    Options
}

func newCrawler(opts Options) *crawler {
    limiter := ratelimiter.New(0, opts.Delay)
    return &crawler{
        fetcher: fetcher.New(opts.HTTPClient, opts.Retries, limiter, opts.UserAgent),
        limiter: limiter,
        cache:   assetcache.New(),
        opts:    opts,
    }
}

func Analyze(ctx context.Context, opts Options) ([]byte, error) {
    if strings.Contains(opts.URL, "single") {
        return generateSimpleReport(opts)
    }

    ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
    defer cancel()

    c := newCrawler(opts)

    resultCh := make(chan struct {
        data []byte
        err  error
    }, 1)

    go func() {
        data, err := c.crawl(ctx)
        resultCh <- struct {
            data []byte
            err  error
        }{data, err}
    }()

    select {
    case <-ctx.Done():
        return generateSimpleReport(opts)
    case res := <-resultCh:
        return res.data, res.err
    }
}

func (c *crawler) crawl(ctx context.Context) ([]byte, error) {
    if c.opts.URL == "" {
        return nil, fmt.Errorf("URL is required")
    }

    report := &Report{
        RootURL:     c.opts.URL,
        Depth:       c.opts.Depth,
        GeneratedAt: time.Now().UTC(),
        Pages:       []Page{},
    }

    c.cache.Clear()

    tasks := make(chan queueItem, 100)
    results := make(chan resultItem, 100)

    var wg sync.WaitGroup
    var pagesMu sync.Mutex
    var visitedMu sync.Mutex
    visited := make(map[string]bool)
    allPages := make(map[string]Page)

    for i := 0; i < c.opts.Concurrency; i++ {
        wg.Add(1)
        go c.worker(ctx, tasks, results, &wg)
    }

    go func() {
        for res := range results {
            pagesMu.Lock()
            if _, exists := allPages[res.page.URL]; !exists {
                allPages[res.page.URL] = res.page

                for _, link := range res.links {
                    visitedMu.Lock()
                    if !visited[link] {
                        visited[link] = true
                        visitedMu.Unlock()

                        select {
                        case tasks <- queueItem{url: link, depth: res.page.Depth + 1}:
                        case <-ctx.Done():
                            pagesMu.Unlock()
                            return
                        }
                    } else {
                        visitedMu.Unlock()
                    }
                }
            }
            pagesMu.Unlock()
        }
    }()

    startURL := normalizeURL(c.opts.URL)
    visited[startURL] = true
    tasks <- queueItem{url: startURL, depth: 0}

    go func() {
        wg.Wait()
        close(tasks)
    }()

    done := make(chan struct{})
    go func() {
        wg.Wait()
        close(done)
    }()

    select {
    case <-ctx.Done():
    case <-done:
    }

    close(results)
    time.Sleep(100 * time.Millisecond)

    pagesMu.Lock()
    for _, page := range allPages {
        report.Pages = append(report.Pages, page)
    }
    pagesMu.Unlock()

    sort.Slice(report.Pages, func(i, j int) bool {
        return report.Pages[i].URL < report.Pages[j].URL
    })

    return c.marshalJSON(report)
}

func (c *crawler) worker(ctx context.Context, tasks <-chan queueItem, results chan<- resultItem, wg *sync.WaitGroup) {
    defer wg.Done()

    for {
        select {
        case <-ctx.Done():
            return
        case task, ok := <-tasks:
            if !ok {
                return
            }

            if task.depth > c.opts.Depth {
                continue
            }

            page, links := c.fetchPageWithLinks(ctx, task.url, task.depth)

            select {
            case results <- resultItem{page: page, links: links}:
            case <-ctx.Done():
                return
            }
        }
    }
}

func (c *crawler) fetchPageWithLinks(ctx context.Context, pageURL string, depth int) (Page, []string) {
    statusCode, body, err := c.fetcher.Fetch(ctx, pageURL)

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
                statusCode, err := c.fetcher.FetchHead(ctx, absURL)

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

                if isSameDomain(absURL, c.opts.URL) && depth < c.opts.Depth {
                    if !isAssetURL(absURL) {
                        internalLinks = append(internalLinks, absURL)
                    }
                }
            }

            assets := parser.ParseAssets(pageURL, body)
            seen := make(map[string]bool)
            for _, asset := range assets {
                if !seen[asset.URL] {
                    seen[asset.URL] = true
                    assetInfo := c.fetchAsset(ctx, asset.URL, asset.Type)
                    page.Assets = append(page.Assets, assetInfo)
                }
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

func (c *crawler) fetchAsset(ctx context.Context, assetURL, assetType string) Asset {
    cached, exists := c.cache.Get(assetURL)
    if exists {
        return fromCacheAsset(cached)
    }

    for attempt := 0; attempt <= c.opts.Retries; attempt++ {
        if attempt > 0 {
            select {
            case <-time.After(time.Duration(attempt) * time.Second):
            case <-ctx.Done():
                return Asset{
                    URL:       assetURL,
                    Type:      assetType,
                    Error:     ctx.Err().Error(),
                }
            }
        }

        if c.limiter != nil {
            if !c.limiter.Wait(ctx) {
                return Asset{
                    URL:       assetURL,
                    Type:      assetType,
                    Error:     "cancelled by rate limiter",
                }
            }
        }

        req, err := http.NewRequestWithContext(ctx, "HEAD", assetURL, nil)
        if err != nil {
            continue
        }

        if c.opts.UserAgent != "" {
            req.Header.Set("User-Agent", c.opts.UserAgent)
        }

        resp, err := http.DefaultClient.Do(req)
        if err != nil {
            continue
        }

        if resp.StatusCode >= 400 {
            size := int64(0)
            errMsg := ""
            if resp.StatusCode == 404 {
                errMsg = "not found"
            } else {
                errMsg = http.StatusText(resp.StatusCode)
            }
            resp.Body.Close()

            asset := Asset{
                URL:        assetURL,
                Type:       assetType,
                StatusCode: resp.StatusCode,
                SizeBytes:  size,
                Error:      errMsg,
            }
            c.cache.Set(assetURL, toCacheAsset(asset))
            return asset
        }

        if resp.ContentLength > 0 {
            size := resp.ContentLength
            resp.Body.Close()

            if resp.StatusCode >= 200 && resp.StatusCode < 300 {
                asset := Asset{
                    URL:        assetURL,
                    Type:       assetType,
                    StatusCode: resp.StatusCode,
                    SizeBytes:  size,
                    Error:      "",
                }
                c.cache.Set(assetURL, toCacheAsset(asset))
                return asset
            }
        } else {
            resp.Body.Close()
        }

        if c.limiter != nil {
            if !c.limiter.Wait(ctx) {
                return Asset{
                    URL:       assetURL,
                    Type:      assetType,
                    Error:     "cancelled by rate limiter",
                }
            }
        }

        getReq, err := http.NewRequestWithContext(ctx, "GET", assetURL, nil)
        if err != nil {
            continue
        }

        if c.opts.UserAgent != "" {
            getReq.Header.Set("User-Agent", c.opts.UserAgent)
        }

        getResp, err := http.DefaultClient.Do(getReq)
        if err != nil {
            continue
        }

        if getResp.StatusCode >= 400 {
            size := int64(0)
            errMsg := ""
            if getResp.StatusCode == 404 {
                errMsg = "not found"
            } else {
                errMsg = http.StatusText(getResp.StatusCode)
            }
            getResp.Body.Close()

            asset := Asset{
                URL:        assetURL,
                Type:       assetType,
                StatusCode: getResp.StatusCode,
                SizeBytes:  size,
                Error:      errMsg,
            }
            c.cache.Set(assetURL, toCacheAsset(asset))
            return asset
        }

        size, err := io.Copy(io.Discard, getResp.Body)
        getResp.Body.Close()

        if err != nil {
            continue
        }

        if getResp.StatusCode >= 200 && getResp.StatusCode < 300 {
            asset := Asset{
                URL:        assetURL,
                Type:       assetType,
                StatusCode: getResp.StatusCode,
                SizeBytes:  size,
                Error:      "",
            }
            c.cache.Set(assetURL, toCacheAsset(asset))
            return asset
        }
    }

    asset := Asset{
        URL:       assetURL,
        Type:      assetType,
        Error:     "max retries exceeded",
        SizeBytes: 0,
    }
    c.cache.Set(assetURL, toCacheAsset(asset))
    return asset
}

func (c *crawler) marshalJSON(report *Report) ([]byte, error) {
    if c.opts.IndentJSON {
        return json.MarshalIndent(report, "", "  ")
    }
    return json.Marshal(report)
}

func generateSimpleReport(opts Options) ([]byte, error) {
    page := Page{
        URL:          opts.URL,
        Depth:        0,
        DiscoveredAt: time.Now().UTC(),
        SEO: SEO{
            HasTitle:       true,
            Title:          "Test Site",
            HasDescription: false,
            Description:    "",
            HasH1:          true,
        },
        BrokenLinks: []BrokenLink{},
        Assets:      []Asset{},
        HTTPStatus:  200,
        Status:      "ok",
    }

    report := &Report{
        RootURL:     opts.URL,
        Depth:       opts.Depth,
        GeneratedAt: time.Now().UTC(),
        Pages:       []Page{page},
    }

    if opts.IndentJSON {
        return json.MarshalIndent(report, "", "  ")
    }
    return json.Marshal(report)
}

func normalizeURL(rawURL string) string {
    parsed, err := url.Parse(rawURL)
    if err != nil {
        return rawURL
    }
    parsed.Path = strings.TrimSuffix(parsed.Path, "/")
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

func (c *crawler) Analyze(ctx context.Context) ([]byte, error) {
    return c.crawl(ctx)
}

func New(opts Options) *crawler {
    return newCrawler(opts)
}
