package crawler

import (
    "context"
    "encoding/json"
    "fmt"
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
        }
    } else {
        page.Status = "error"
        page.Error = http.StatusText(statusCode)
        page.BrokenLinks = nil
        page.Assets = nil
    }

    return page, internalLinks
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

func New(opts Options) *crawler {
    return &crawler{
        fetcher: fetcher.New(opts.HTTPClient, opts.Retries, nil, opts.UserAgent),
        limiter: ratelimiter.New(0, opts.Delay),
        cache:   assetcache.New(),
        opts:    opts,
    }
}

func (c *crawler) Analyze(ctx context.Context) ([]byte, error) {
    return Analyze(ctx, c.opts)
}