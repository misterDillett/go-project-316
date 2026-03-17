package parser

import (
    "html"
    "net/url"
    "strings"

    "github.com/PuerkitoBio/goquery"
)

type Link struct {
    URL string
}

type SEOTags struct {
    Title       string
    Description string
    H1          string
}

type AssetInfo struct {
    URL  string
    Type string
}

func ParseLinks(baseURL string, htmlContent []byte) ([]Link, error) {
    doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(htmlContent)))
    if err != nil {
        return nil, err
    }

    var links []Link
    seen := make(map[string]bool)

    doc.Find("a[href]").Each(func(i int, s *goquery.Selection) {
        href, exists := s.Attr("href")
        if !exists || href == "" || strings.HasPrefix(href, "#") {
            return
        }

        absoluteURL := resolveRelativeURL(baseURL, href)
        if absoluteURL == "" {
            return
        }

        if !isSchemeSupported(absoluteURL) {
            return
        }

        if !seen[absoluteURL] {
            seen[absoluteURL] = true
            links = append(links, Link{URL: absoluteURL})
        }
    })

    return links, nil
}

func ParseAssets(baseURL string, htmlContent []byte) []AssetInfo {
    doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(htmlContent)))
    if err != nil {
        return nil
    }

    var assets []AssetInfo
    seen := make(map[string]bool)

    doc.Find("img[src]").Each(func(i int, s *goquery.Selection) {
        src, exists := s.Attr("src")
        if !exists || src == "" {
            return
        }

        absoluteURL := resolveRelativeURL(baseURL, src)
        if absoluteURL == "" || !isSchemeSupported(absoluteURL) {
            return
        }

        if !seen[absoluteURL] {
            seen[absoluteURL] = true
            assets = append(assets, AssetInfo{URL: absoluteURL, Type: "image"})
        }
    })

    doc.Find("script[src]").Each(func(i int, s *goquery.Selection) {
        src, exists := s.Attr("src")
        if !exists || src == "" {
            return
        }

        absoluteURL := resolveRelativeURL(baseURL, src)
        if absoluteURL == "" || !isSchemeSupported(absoluteURL) {
            return
        }

        if !seen[absoluteURL] {
            seen[absoluteURL] = true
            assets = append(assets, AssetInfo{URL: absoluteURL, Type: "script"})
        }
    })

    doc.Find("link[rel='stylesheet'][href]").Each(func(i int, s *goquery.Selection) {
        href, exists := s.Attr("href")
        if !exists || href == "" {
            return
        }

        absoluteURL := resolveRelativeURL(baseURL, href)
        if absoluteURL == "" || !isSchemeSupported(absoluteURL) {
            return
        }

        if !seen[absoluteURL] {
            seen[absoluteURL] = true
            assets = append(assets, AssetInfo{URL: absoluteURL, Type: "style"})
        }
    })

    return assets
}

func ParseSEOTags(htmlContent []byte) SEOTags {
    var seo SEOTags

    doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(htmlContent)))
    if err != nil {
        return seo
    }

    doc.Find("title").Each(func(i int, s *goquery.Selection) {
        if i == 0 {
            seo.Title = cleanText(s.Text())
        }
    })

    doc.Find("meta[name='description']").Each(func(i int, s *goquery.Selection) {
        if content, exists := s.Attr("content"); exists {
            seo.Description = cleanText(content)
        }
    })

    doc.Find("h1").Each(func(i int, s *goquery.Selection) {
        if i == 0 {
            seo.H1 = cleanText(s.Text())
        }
    })

    return seo
}

func cleanText(text string) string {
    text = html.UnescapeString(text)
    text = strings.TrimSpace(text)
    for strings.Contains(text, "  ") {
        text = strings.ReplaceAll(text, "  ", " ")
    }
    return text
}

func resolveRelativeURL(base, ref string) string {
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

func isSchemeSupported(rawURL string) bool {
    parsed, err := url.Parse(rawURL)
    if err != nil {
        return false
    }

    scheme := strings.ToLower(parsed.Scheme)
    return scheme == "http" || scheme == "https" || scheme == ""
}