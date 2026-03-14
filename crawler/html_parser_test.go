package crawler

import (
    "testing"
)

func TestParseLinks_Basic(t *testing.T) {
    html := `
    <html>
        <body>
            <a href="/page1">Page 1</a>
            <a href="https://example.com/page2">Page 2</a>
            <a href="#anchor">Skip this</a>
            <a href="">Empty</a>
        </body>
    </html>
    `

    links, err := ParseLinks("https://example.com", []byte(html))
    if err != nil {
        t.Fatalf("Expected no error, got %v", err)
    }

    expected := 2
    if len(links) != expected {
        t.Errorf("Expected %d links, got %d", expected, len(links))
    }

    found := false
    for _, link := range links {
        if link.URL == "https://example.com/page1" {
            found = true
            break
        }
    }
    if !found {
        t.Error("Expected to find https://example.com/page1")
    }
}

func TestParseLinks_IgnoreUnsupported(t *testing.T) {
    html := `
    <html>
        <body>
            <a href="mailto:test@example.com">Email</a>
            <a href="ftp://example.com/file">FTP</a>
            <a href="javascript:void(0)">JavaScript</a>
            <a href="http://example.com">HTTP</a>
            <a href="https://example.com">HTTPS</a>
        </body>
    </html>
    `

    links, err := ParseLinks("https://example.com", []byte(html))
    if err != nil {
        t.Fatalf("Expected no error, got %v", err)
    }

    expected := 2
    if len(links) != expected {
        t.Errorf("Expected %d links, got %d", expected, len(links))
    }
}

func TestParseLinks_Duplicates(t *testing.T) {
    html := `
    <html>
        <body>
            <a href="/page1">Page 1</a>
            <a href="https://example.com/page1">Same Page 1</a>
            <a href="/page2">Page 2</a>
        </body>
    </html>
    `

    links, err := ParseLinks("https://example.com", []byte(html))
    if err != nil {
        t.Fatalf("Expected no error, got %v", err)
    }

    expected := 2
    if len(links) != expected {
        t.Errorf("Expected %d unique links, got %d", expected, len(links))
    }
}

func TestParseSEOTags_AllPresent(t *testing.T) {
    html := `
    <html>
        <head>
            <title>Test Title &amp; More</title>
            <meta name="description" content="Test Description with &amp; entity">
        </head>
        <body>
            <h1>Test H1 with special chars &amp; </h1>
        </body>
    </html>
    `

    seo := ParseSEOTags([]byte(html))

    if seo.Title != "Test Title & More" {
        t.Errorf("Expected title 'Test Title & More', got '%s'", seo.Title)
    }

    if seo.Description != "Test Description with & entity" {
        t.Errorf("Expected description 'Test Description with & entity', got '%s'", seo.Description)
    }

    if seo.H1 != "Test H1 with special chars &" {
        t.Errorf("Expected h1 'Test H1 with special chars &', got '%s'", seo.H1)
    }
}

func TestParseSEOTags_Missing(t *testing.T) {
    html := `
    <html>
        <head></head>
        <body></body>
    </html>
    `

    seo := ParseSEOTags([]byte(html))

    if seo.Title != "" {
        t.Errorf("Expected empty title, got '%s'", seo.Title)
    }
    if seo.Description != "" {
        t.Errorf("Expected empty description, got '%s'", seo.Description)
    }
    if seo.H1 != "" {
        t.Errorf("Expected empty h1, got '%s'", seo.H1)
    }
}

func TestParseSEOTags_MultipleH1(t *testing.T) {
    html := `
    <html>
        <body>
            <h1>First H1</h1>
            <h1>Second H1</h1>
        </body>
    </html>
    `

    seo := ParseSEOTags([]byte(html))

    if seo.H1 != "First H1" {
        t.Errorf("Expected first h1 'First H1', got '%s'", seo.H1)
    }
}

func TestResolveRelativeURL(t *testing.T) {
    tests := []struct {
        base     string
        ref      string
        expected string
    }{
        {"https://example.com", "/page", "https://example.com/page"},
        {"https://example.com/", "page", "https://example.com/page"},
        {"https://example.com/path/", "../other", "https://example.com/other"},
        {"https://example.com", "https://other.com", "https://other.com"},
    }

    for _, test := range tests {
        result := resolveRelativeURL(test.base, test.ref)
        if result != test.expected {
            t.Errorf("resolveRelativeURL(%s, %s) = %s; expected %s",
                test.base, test.ref, result, test.expected)
        }
    }
}