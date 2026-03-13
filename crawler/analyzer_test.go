package crawler

import (
    "context"
    "encoding/json"
    "errors"
    "io"
    "net/http"
    "strings"
    "testing"
    "time"
)

type MockHTTPClient struct {
    Responses map[string]*http.Response
    Errors    map[string]error
    DefaultResponse *http.Response
    DefaultError    error
}

func NewMockHTTPClient() *MockHTTPClient {
    return &MockHTTPClient{
        Responses: make(map[string]*http.Response),
        Errors:    make(map[string]error),
    }
}

func (m *MockHTTPClient) Do(req *http.Request) (*http.Response, error) {
    url := req.URL.String()

    if resp, ok := m.Responses[url]; ok {
        return resp, nil
    }
    if err, ok := m.Errors[url]; ok {
        return nil, err
    }

    if m.DefaultError != nil {
        return nil, m.DefaultError
    }
    if m.DefaultResponse != nil {
        return m.DefaultResponse, nil
    }

    return &http.Response{
        StatusCode: 200,
        Status:     "200 OK",
        Body:       io.NopCloser(strings.NewReader("OK")),
    }, nil
}

func TestAnalyze_WithSEO(t *testing.T) {
    mockClient := NewMockHTTPClient()

    html := `
    <html>
        <head>
            <title>Test Title</title>
            <meta name="description" content="Test Description">
        </head>
        <body>
            <h1>Test H1</h1>
            <a href="/page1">Link 1</a>
        </body>
    </html>
    `

    mockClient.Responses["https://example.com"] = &http.Response{
        StatusCode: 200,
        Status:     "200 OK",
        Header:     http.Header{"Content-Type": []string{"text/html"}},
        Body:       io.NopCloser(strings.NewReader(html)),
    }

    opts := Options{
        URL:        "https://example.com",
        Depth:      1,
        Timeout:    5 * time.Second,
        HTTPClient: mockClient,
    }

    result, err := Analyze(context.Background(), opts)
    if err != nil {
        t.Fatalf("Expected no error, got %v", err)
    }

    var report Report
    if err := json.Unmarshal(result, &report); err != nil {
        t.Fatalf("Failed to unmarshal result: %v", err)
    }

    if len(report.Pages) != 1 {
        t.Fatalf("Expected 1 page, got %d", len(report.Pages))
    }

    page := report.Pages[0]

    if !page.SEO.HasTitle {
        t.Error("Expected HasTitle true")
    }
    if page.SEO.Title != "Test Title" {
        t.Errorf("Expected title 'Test Title', got '%s'", page.SEO.Title)
    }

    if !page.SEO.HasDescription {
        t.Error("Expected HasDescription true")
    }
    if page.SEO.Description != "Test Description" {
        t.Errorf("Expected description 'Test Description', got '%s'", page.SEO.Description)
    }

    if !page.SEO.HasH1 {
        t.Error("Expected HasH1 true")
    }
    if page.SEO.H1 != "Test H1" {
        t.Errorf("Expected h1 'Test H1', got '%s'", page.SEO.H1)
    }
}

func TestAnalyze_WithoutSEO(t *testing.T) {
    mockClient := NewMockHTTPClient()

    html := `<html><body>No SEO here</body></html>`

    mockClient.Responses["https://example.com"] = &http.Response{
        StatusCode: 200,
        Status:     "200 OK",
        Header:     http.Header{"Content-Type": []string{"text/html"}},
        Body:       io.NopCloser(strings.NewReader(html)),
    }

    opts := Options{
        URL:        "https://example.com",
        Depth:      1,
        Timeout:    5 * time.Second,
        HTTPClient: mockClient,
    }

    result, err := Analyze(context.Background(), opts)
    if err != nil {
        t.Fatalf("Expected no error, got %v", err)
    }

    var report Report
    if err := json.Unmarshal(result, &report); err != nil {
        t.Fatalf("Failed to unmarshal result: %v", err)
    }

    page := report.Pages[0]

    if page.SEO.HasTitle {
        t.Error("Expected HasTitle false")
    }
    if page.SEO.Title != "" {
        t.Errorf("Expected empty title, got '%s'", page.SEO.Title)
    }

    if page.SEO.HasDescription {
        t.Error("Expected HasDescription false")
    }
    if page.SEO.Description != "" {
        t.Errorf("Expected empty description, got '%s'", page.SEO.Description)
    }

    if page.SEO.HasH1 {
        t.Error("Expected HasH1 false")
    }
    if page.SEO.H1 != "" {
        t.Errorf("Expected empty h1, got '%s'", page.SEO.H1)
    }
}

func TestAnalyze_WithBrokenLinks(t *testing.T) {
    mockClient := NewMockHTTPClient()

    html := `
    <html>
        <body>
            <a href="/good">Good Link</a>
            <a href="/bad">Bad Link</a>
        </body>
    </html>
    `

    mockClient.Responses["https://example.com"] = &http.Response{
        StatusCode: 200,
        Status:     "200 OK",
        Header:     http.Header{"Content-Type": []string{"text/html"}},
        Body:       io.NopCloser(strings.NewReader(html)),
    }

    mockClient.Responses["https://example.com/good"] = &http.Response{
        StatusCode: 200,
        Status:     "200 OK",
        Body:       io.NopCloser(strings.NewReader("Good")),
    }

    mockClient.Responses["https://example.com/bad"] = &http.Response{
        StatusCode: 404,
        Status:     "404 Not Found",
        Body:       io.NopCloser(strings.NewReader("Not Found")),
    }

    opts := Options{
        URL:        "https://example.com",
        Depth:      1,
        Timeout:    5 * time.Second,
        HTTPClient: mockClient,
    }

    result, err := Analyze(context.Background(), opts)
    if err != nil {
        t.Fatalf("Expected no error, got %v", err)
    }

    var report Report
    if err := json.Unmarshal(result, &report); err != nil {
        t.Fatalf("Failed to unmarshal result: %v", err)
    }

    if len(report.Pages) != 1 {
        t.Fatalf("Expected 1 page, got %d", len(report.Pages))
    }

    page := report.Pages[0]
    if len(page.BrokenLinks) != 1 {
        t.Fatalf("Expected 1 broken link, got %d", len(page.BrokenLinks))
    }

    broken := page.BrokenLinks[0]
    if broken.URL != "https://example.com/bad" {
        t.Errorf("Expected broken URL example.com/bad, got %s", broken.URL)
    }
    if broken.StatusCode != 404 {
        t.Errorf("Expected status code 404, got %d", broken.StatusCode)
    }
}

func TestAnalyze_WithNetworkErrorInLink(t *testing.T) {
    mockClient := NewMockHTTPClient()

    html := `
    <html>
        <body>
            <a href="/good">Good Link</a>
            <a href="/timeout">Timeout Link</a>
        </body>
    </html>
    `

    mockClient.Responses["https://example.com"] = &http.Response{
        StatusCode: 200,
        Status:     "200 OK",
        Header:     http.Header{"Content-Type": []string{"text/html"}},
        Body:       io.NopCloser(strings.NewReader(html)),
    }

    mockClient.Responses["https://example.com/good"] = &http.Response{
        StatusCode: 200,
        Status:     "200 OK",
        Body:       io.NopCloser(strings.NewReader("Good")),
    }

    mockClient.Errors["https://example.com/timeout"] = errors.New("connection timeout")

    opts := Options{
        URL:        "https://example.com",
        Depth:      1,
        Timeout:    5 * time.Second,
        HTTPClient: mockClient,
    }

    result, err := Analyze(context.Background(), opts)
    if err != nil {
        t.Fatalf("Expected no error, got %v", err)
    }

    var report Report
    if err := json.Unmarshal(result, &report); err != nil {
        t.Fatalf("Failed to unmarshal result: %v", err)
    }

    page := report.Pages[0]
    if len(page.BrokenLinks) != 1 {
        t.Fatalf("Expected 1 broken link, got %d", len(page.BrokenLinks))
    }

    broken := page.BrokenLinks[0]
    if broken.URL != "https://example.com/timeout" {
        t.Errorf("Expected broken URL example.com/timeout, got %s", broken.URL)
    }
    if broken.Error == "" {
        t.Error("Expected error message, got empty")
    }
}

func TestAnalyze_NoBrokenLinks(t *testing.T) {
    mockClient := NewMockHTTPClient()

    html := `
    <html>
        <body>
            <a href="/good1">Good Link 1</a>
            <a href="/good2">Good Link 2</a>
        </body>
    </html>
    `

    mockClient.Responses["https://example.com"] = &http.Response{
        StatusCode: 200,
        Status:     "200 OK",
        Header:     http.Header{"Content-Type": []string{"text/html"}},
        Body:       io.NopCloser(strings.NewReader(html)),
    }

    mockClient.Responses["https://example.com/good1"] = &http.Response{
        StatusCode: 200,
        Status:     "200 OK",
        Body:       io.NopCloser(strings.NewReader("Good 1")),
    }

    mockClient.Responses["https://example.com/good2"] = &http.Response{
        StatusCode: 200,
        Status:     "200 OK",
        Body:       io.NopCloser(strings.NewReader("Good 2")),
    }

    opts := Options{
        URL:        "https://example.com",
        Depth:      1,
        Timeout:    5 * time.Second,
        HTTPClient: mockClient,
    }

    result, err := Analyze(context.Background(), opts)
    if err != nil {
        t.Fatalf("Expected no error, got %v", err)
    }

    var report Report
    if err := json.Unmarshal(result, &report); err != nil {
        t.Fatalf("Failed to unmarshal result: %v", err)
    }

    page := report.Pages[0]
    if len(page.BrokenLinks) != 0 {
        t.Errorf("Expected 0 broken links, got %d", len(page.BrokenLinks))
    }
}

func TestAnalyze_NonHTMLContent(t *testing.T) {
    mockClient := NewMockHTTPClient()

    mockClient.Responses["https://example.com"] = &http.Response{
        StatusCode: 200,
        Status:     "200 OK",
        Header:     http.Header{"Content-Type": []string{"application/json"}},
        Body:       io.NopCloser(strings.NewReader(`{"key": "value"}`)),
    }

    opts := Options{
        URL:        "https://example.com",
        Depth:      1,
        Timeout:    5 * time.Second,
        HTTPClient: mockClient,
    }

    result, err := Analyze(context.Background(), opts)
    if err != nil {
        t.Fatalf("Expected no error, got %v", err)
    }

    var report Report
    if err := json.Unmarshal(result, &report); err != nil {
        t.Fatalf("Failed to unmarshal result: %v", err)
    }

    page := report.Pages[0]

    if page.SEO.HasTitle {
        t.Error("Expected HasTitle false for non-HTML")
    }
    if len(page.BrokenLinks) != 0 {
        t.Errorf("Expected 0 broken links for non-HTML content, got %d", len(page.BrokenLinks))
    }
}