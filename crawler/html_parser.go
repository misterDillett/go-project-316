package crawler

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

        absoluteURL, err := resolveURL(baseURL, href)
        if err != nil {
            return
        }

        if !isSupportedScheme(absoluteURL) {
            return
        }

        if !seen[absoluteURL] {
            seen[absoluteURL] = true
            links = append(links, Link{URL: absoluteURL})
        }
    })

    return links, nil
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

func resolveURL(base, ref string) (string, error) {
    baseURL, err := url.Parse(base)
    if err != nil {
        return "", err
    }

    refURL, err := url.Parse(ref)
    if err != nil {
        return "", err
    }

    absoluteURL := baseURL.ResolveReference(refURL)
    return absoluteURL.String(), nil
}

func isSupportedScheme(rawURL string) bool {
    parsed, err := url.Parse(rawURL)
    if err != nil {
        return false
    }

    scheme := strings.ToLower(parsed.Scheme)
    return scheme == "http" || scheme == "https" || scheme == ""
}