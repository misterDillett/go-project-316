package crawler

import (
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "strings"
    "time"
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

    page, err := fetchPage(ctx, opts, opts.URL, 0)
    if err != nil {
        page = Page{
            URL:          opts.URL,
            Depth:        0,
            HTTPStatus:   0,
            Status:       "error",
            Error:        err.Error(),
            SEO:          SEO{},
            BrokenLinks:  []BrokenLink{},
            DiscoveredAt: time.Now().UTC(),
        }
    }

    report.Pages = append(report.Pages, page)

    var jsonData []byte
    var marshalErr error

    if opts.IndentJSON {
        jsonData, marshalErr = json.MarshalIndent(report, "", "  ")
    } else {
        jsonData, marshalErr = json.Marshal(report)
    }

    if marshalErr != nil {
        return nil, fmt.Errorf("failed to marshal report: %w", marshalErr)
    }

    return jsonData, nil
}

func fetchPage(ctx context.Context, opts Options, pageURL string, depth int) (Page, error) {
    page := Page{
        URL:          pageURL,
        Depth:        depth,
        DiscoveredAt: time.Now().UTC(),
        SEO:          SEO{},
        BrokenLinks:  []BrokenLink{},
    }

    req, err := http.NewRequestWithContext(ctx, "GET", pageURL, nil)
    if err != nil {
        page.HTTPStatus = 0
        page.Status = "error"
        page.Error = err.Error()
        return page, err
    }

    if opts.UserAgent != "" {
        req.Header.Set("User-Agent", opts.UserAgent)
    }

    resp, err := opts.HTTPClient.Do(req)
    if err != nil {
        page.HTTPStatus = 0
        page.Status = "error"
        page.Error = err.Error()
        return page, err
    }
    defer resp.Body.Close()

    body, err := io.ReadAll(resp.Body)
    if err != nil {
        page.HTTPStatus = resp.StatusCode
        page.Status = "error"
        page.Error = err.Error()
        return page, err
    }

    page.HTTPStatus = resp.StatusCode
    if resp.StatusCode >= 200 && resp.StatusCode < 300 {
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
            if err == nil && len(links) > 0 {
                page.BrokenLinks = checkLinks(ctx, opts, links)
            }
        }
    } else {
        page.Status = "error"
        page.Error = http.StatusText(resp.StatusCode)
    }

    return page, nil
}

func isHTMLContent(contentType string) bool {
    return contentType == "" ||
        contentType == "text/html" ||
        strings.Contains(contentType, "text/html")
}

func checkLinks(ctx context.Context, opts Options, links []Link) []BrokenLink {
    var broken []BrokenLink

    for _, link := range links {
        statusCode, err := checkLink(ctx, opts, link.URL)

        brokenLink := BrokenLink{
            URL: link.URL,
        }

        if err != nil {
            brokenLink.Error = err.Error()
            broken = append(broken, brokenLink)
        } else if statusCode >= 400 {
            brokenLink.StatusCode = statusCode
            broken = append(broken, brokenLink)
        }
    }

    return broken
}

func checkLink(ctx context.Context, opts Options, linkURL string) (int, error) {
    req, err := http.NewRequestWithContext(ctx, "HEAD", linkURL, nil)
    if err != nil {
        return 0, err
    }

    if opts.UserAgent != "" {
        req.Header.Set("User-Agent", opts.UserAgent)
    }

    resp, err := opts.HTTPClient.Do(req)
    if err != nil {
        return 0, err
    }
    defer resp.Body.Close()

    if resp.StatusCode == 405 {
        return checkLinkWithGET(ctx, opts, linkURL)
    }

    return resp.StatusCode, nil
}

func checkLinkWithGET(ctx context.Context, opts Options, linkURL string) (int, error) {
    req, err := http.NewRequestWithContext(ctx, "GET", linkURL, nil)
    if err != nil {
        return 0, err
    }

    if opts.UserAgent != "" {
        req.Header.Set("User-Agent", opts.UserAgent)
    }

    resp, err := opts.HTTPClient.Do(req)
    if err != nil {
        return 0, err
    }
    defer resp.Body.Close()

    _, err = io.CopyN(io.Discard, resp.Body, 1024)
    if err != nil && err != io.EOF {
        return 0, err
    }

    return resp.StatusCode, nil
}