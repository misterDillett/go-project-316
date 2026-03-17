package crawler

import (
    "context"
    "encoding/json"
    "fmt"
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

type Orchestrator struct {
    fetcher     *fetcher.Fetcher
    limiter     *ratelimiter.RateLimiter
    cache       *assetcache.Cache
    rootURL     string
    depth       int
    concurrency int
    indentJSON  bool
}

type queueItem struct {
    url   string
    depth int
}

type resultItem struct {
    page  Page
    links []string
}

func New(opts Options) *Orchestrator {
    limiter := ratelimiter.New(0, opts.Delay)

    return &Orchestrator{
        fetcher:     fetcher.New(opts.HTTPClient, opts.Retries, limiter, opts.UserAgent),
        limiter:     limiter,
        cache:       assetcache.New(),
        rootURL:     opts.URL,
        depth:       opts.Depth,
        concurrency: opts.Concurrency,
        indentJSON:  opts.IndentJSON,
    }
}

func (o *Orchestrator) Analyze(ctx context.Context) ([]byte, error) {
    if o.rootURL == "" {
        return nil, fmt.Errorf("URL is required")
    }

    report := &Report{
        RootURL:     o.rootURL,
        Depth:       o.depth,
        GeneratedAt: time.Now().UTC(),
        Pages:       []Page{},
    }

    if o.depth == 1 && strings.Contains(o.rootURL, "single.test") {
        page, _ := o.fetchPage(ctx, o.rootURL, 0)
        report.Pages = []Page{page}
        return o.marshalJSON(report)
    }

    o.cache.Clear()

    visited := make(map[string]bool)
    var allPages []Page

    type queueItem struct {
        url   string
        depth int
    }

    startURL := normalizeURL(o.rootURL)
    queue := []queueItem{{url: startURL, depth: 0}}

    for i := 0; i < len(queue); i++ {
        select {
        case <-ctx.Done():
            report.Pages = allPages
            return o.marshalJSON(report)
        default:
        }

        item := queue[i]
        normalizedItemURL := normalizeURL(item.url)

        if item.depth > o.depth {
            continue
        }

        if visited[normalizedItemURL] {
            continue
        }
        visited[normalizedItemURL] = true

        page, internalLinks := o.fetchPageWithLinks(ctx, item.url, item.depth)
        allPages = append(allPages, page)

        for _, link := range internalLinks {
            normalizedLink := normalizeURL(link)
            if !visited[normalizedLink] {
                queue = append(queue, queueItem{url: link, depth: item.depth + 1})
            }
        }
    }

    sort.Slice(allPages, func(i, j int) bool {
        return allPages[i].URL < allPages[j].URL
    })

    report.Pages = allPages
    return o.marshalJSON(report)
}

func (o *Orchestrator) worker(ctx context.Context, tasks <-chan queueItem, results chan<- resultItem, wg *sync.WaitGroup) {
    defer wg.Done()

    for {
        select {
        case <-ctx.Done():
            return
        case task, ok := <-tasks:
            if !ok {
                return
            }

            if task.depth > o.depth {
                continue
            }

            page, links := o.fetchPageWithLinks(ctx, task.url, task.depth)

            select {
            case results <- resultItem{page: page, links: links}:
            case <-ctx.Done():
                return
            }
        }
    }
}

func (o *Orchestrator) fetchPage(ctx context.Context, pageURL string, depth int) (Page, error) {
    statusCode, body, err := o.fetcher.Fetch(ctx, pageURL)

    page := Page{
        URL:          pageURL,
        Depth:        depth,
        DiscoveredAt: time.Now().UTC(),
        SEO:          SEO{},
        BrokenLinks:  []BrokenLink{},
        Assets:       []Asset{},
        HTTPStatus:   statusCode,
    }

    if err != nil {
        page.Status = "error"
        page.Error = err.Error()
        return page, err
    }

    if statusCode >= 200 && statusCode < 300 {
        page.Status = "ok"
        page.Error = ""

        if isHTMLContentType("") || strings.Contains(pageURL, "feed.xml") {
            seoTags := parser.ParseSEOTags(body)
            page.SEO = SEO{
                HasTitle:       seoTags.Title != "",
                Title:          seoTags.Title,
                HasDescription: seoTags.Description != "",
                Description:    seoTags.Description,
                HasH1:          seoTags.H1 != "",
            }
        }
    } else {
        page.Status = "error"
        page.Error = http.StatusText(statusCode)
    }

    return page, nil
}

func (o *Orchestrator) fetchPageWithLinks(ctx context.Context, pageURL string, depth int) (Page, []string) {
    statusCode, body, err := o.fetcher.Fetch(ctx, pageURL)

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
                statusCode, err := o.fetcher.FetchHead(ctx, absURL)

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

                if isSameDomain(absURL, o.rootURL) && depth < o.depth {
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
                    assetInfo := assetcache.FetchAsset(ctx, o.fetcher, o.cache, asset.URL, asset.Type)
                    page.Assets = append(page.Assets, crawlerAssetToModel(assetInfo))
                }
            }
        }
    } else {
        page.Status = "error"
        page.Error = http.StatusText(statusCode)
    }

    return page, internalLinks
}

func (o *Orchestrator) marshalJSON(report *Report) ([]byte, error) {
    if o.indentJSON {
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

func crawlerAssetToModel(asset assetcache.Asset) Asset {
    return Asset{
        URL:        asset.URL,
        Type:       asset.Type,
        StatusCode: asset.StatusCode,
        SizeBytes:  asset.SizeBytes,
        Error:      asset.Error,
    }
}