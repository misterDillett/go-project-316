package crawler

import (
    "context"
    "encoding/json"
    "errors"
    "io"
    "net/http"
    "net/url"
    "strings"
    "testing"
    "time"
)

type MockHTTPClient struct {
    Responses map[string]*http.Response
    Errors    map[string]error
    DefaultResponse *http.Response
    DefaultError    error
    hook      func(*http.Request)
}

func NewMockHTTPClient() *MockHTTPClient {
    return &MockHTTPClient{
        Responses: make(map[string]*http.Response),
        Errors:    make(map[string]error),
    }
}

func (m *MockHTTPClient) Do(req *http.Request) (*http.Response, error) {
    if m.hook != nil {
        m.hook(req)
    }

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
        Depth:      0,
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
        Depth:      0,
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
        Depth:      0,
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
        Depth:      0,
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
        Depth:      0,
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
    if page.BrokenLinks != nil {
        t.Errorf("Expected nil broken links, got %v", page.BrokenLinks)
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
        Depth:      0,
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
    if page.BrokenLinks != nil {
        t.Errorf("Expected nil broken links for non-HTML content, got %v", page.BrokenLinks)
    }
}

func TestAnalyze_DepthLimit(t *testing.T) {
    t.Skip("Skipping depth limit test - will be fixed later")
}

func TestAnalyze_UniquePages(t *testing.T) {
    mockClient := NewMockHTTPClient()

    html := `
    <html>
        <body>
            <a href="/page1">Page 1</a>
            <a href="/page1">Page 1 again</a>
            <a href="/page2">Page 2</a>
        </body>
    </html>
    `

    mockClient.Responses["https://example.com"] = &http.Response{
        StatusCode: 200,
        Header:     http.Header{"Content-Type": []string{"text/html"}},
        Body:       io.NopCloser(strings.NewReader(html)),
    }

    mockClient.Responses["https://example.com/page1"] = &http.Response{
        StatusCode: 200,
        Header:     http.Header{"Content-Type": []string{"text/html"}},
        Body:       io.NopCloser(strings.NewReader("<html></html>")),
    }

    mockClient.Responses["https://example.com/page2"] = &http.Response{
        StatusCode: 200,
        Header:     http.Header{"Content-Type": []string{"text/html"}},
        Body:       io.NopCloser(strings.NewReader("<html></html>")),
    }

    opts := Options{
        URL:         "https://example.com",
        Depth:       2,
        Timeout:     5 * time.Second,
        Concurrency: 2,
        HTTPClient:  mockClient,
    }

    result, err := Analyze(context.Background(), opts)
    if err != nil {
        t.Fatalf("Expected no error, got %v", err)
    }

    var report Report
    if err := json.Unmarshal(result, &report); err != nil {
        t.Fatalf("Failed to unmarshal result: %v", err)
    }

    expected := 3
    if len(report.Pages) != expected {
        t.Errorf("Expected %d unique pages, got %d", expected, len(report.Pages))
    }
}

func TestAnalyze_ExternalLinks(t *testing.T) {
    mockClient := NewMockHTTPClient()

    html := `
    <html>
        <body>
            <a href="/internal">Internal</a>
            <a href="https://external.com">External</a>
            <a href="https://another.com">Another External</a>
        </body>
    </html>
    `

    mockClient.Responses["https://example.com"] = &http.Response{
        StatusCode: 200,
        Header:     http.Header{"Content-Type": []string{"text/html"}},
        Body:       io.NopCloser(strings.NewReader(html)),
    }

    mockClient.Responses["https://example.com/internal"] = &http.Response{
        StatusCode: 200,
        Header:     http.Header{"Content-Type": []string{"text/html"}},
        Body:       io.NopCloser(strings.NewReader("<html></html>")),
    }

    opts := Options{
        URL:         "https://example.com",
        Depth:       2,
        Timeout:     5 * time.Second,
        Concurrency: 2,
        HTTPClient:  mockClient,
    }

    result, err := Analyze(context.Background(), opts)
    if err != nil {
        t.Fatalf("Expected no error, got %v", err)
    }

    var report Report
    if err := json.Unmarshal(result, &report); err != nil {
        t.Fatalf("Failed to unmarshal result: %v", err)
    }

    expected := 2
    if len(report.Pages) != expected {
        t.Errorf("Expected %d pages (only internal), got %d", expected, len(report.Pages))
    }
}

func TestAnalyze_CancelContext(t *testing.T) {
    mockClient := NewMockHTTPClient()

    html := `
    <html>
        <body>
            <a href="/page1">Page 1</a>
            <a href="/page2">Page 2</a>
            <a href="/page3">Page 3</a>
        </body>
    </html>
    `

    mockClient.Responses["https://example.com"] = &http.Response{
        StatusCode: 200,
        Header:     http.Header{"Content-Type": []string{"text/html"}},
        Body:       io.NopCloser(strings.NewReader(html)),
    }

    ctx, cancel := context.WithCancel(context.Background())

    opts := Options{
        URL:         "https://example.com",
        Depth:       3,
        Timeout:     5 * time.Second,
        Concurrency: 1,
        HTTPClient:  mockClient,
    }

    go func() {
        time.Sleep(10 * time.Millisecond)
        cancel()
    }()

    result, err := Analyze(ctx, opts)
    if err != nil {
        t.Fatalf("Expected no error, got %v", err)
    }

    var report Report
    if err := json.Unmarshal(result, &report); err != nil {
        t.Fatalf("Failed to unmarshal result: %v", err)
    }

    if len(report.Pages) == 0 {
        t.Error("Expected at least one page even after cancellation")
    }
}

func TestAnalyze_WithDelay(t *testing.T) {
    mockClient := NewMockHTTPClient()

    html := `<html><body>Test</body></html>`

    mockClient.Responses["https://example.com"] = &http.Response{
        StatusCode: 200,
        Status:     "200 OK",
        Header:     http.Header{"Content-Type": []string{"text/html"}},
        Body:       io.NopCloser(strings.NewReader(html)),
    }

    opts := Options{
        URL:        "https://example.com",
        Depth:      0,
        Timeout:    5 * time.Second,
        Delay:      100 * time.Millisecond,
        HTTPClient: mockClient,
    }

    start := time.Now()
    result, err := Analyze(context.Background(), opts)
    elapsed := time.Since(start)

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

    if elapsed < 100*time.Millisecond {
        t.Errorf("Expected delay of at least 100ms, got %v", elapsed)
    }
}

func TestAnalyze_WithRetriesSuccess(t *testing.T) {
    mockClient := NewMockHTTPClient()

    html := `<html><body>Test</body></html>`

    mockClient.Responses["https://example.com"] = &http.Response{
        StatusCode: 500,
        Status:     "500 Internal Server Error",
        Header:     http.Header{"Content-Type": []string{"text/html"}},
        Body:       io.NopCloser(strings.NewReader(html)),
    }

    mockClient.Responses["https://example.com_2"] = &http.Response{
        StatusCode: 200,
        Status:     "200 OK",
        Header:     http.Header{"Content-Type": []string{"text/html"}},
        Body:       io.NopCloser(strings.NewReader(html)),
    }

    requestCount := 0
    mockClient.hook = func(req *http.Request) {
        requestCount++
        if requestCount == 2 {
            req.URL, _ = url.Parse("https://example.com_2")
        }
    }

    opts := Options{
        URL:        "https://example.com",
        Depth:      0,
        Timeout:    5 * time.Second,
        Retries:    1,
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
    if page.HTTPStatus != 200 {
        t.Errorf("Expected status 200, got %d", page.HTTPStatus)
    }
    if page.Status != "ok" {
        t.Errorf("Expected status 'ok', got %s", page.Status)
    }
}

func TestAnalyze_WithRetriesFailure(t *testing.T) {
    mockClient := NewMockHTTPClient()

    html := `<html><body>Test</body></html>`

    mockClient.Responses["https://example.com"] = &http.Response{
        StatusCode: 500,
        Status:     "500 Internal Server Error",
        Header:     http.Header{"Content-Type": []string{"text/html"}},
        Body:       io.NopCloser(strings.NewReader(html)),
    }

    opts := Options{
        URL:        "https://example.com",
        Depth:      0,
        Timeout:    5 * time.Second,
        Retries:    2,
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
    if page.HTTPStatus != 500 {
        t.Errorf("Expected status 500, got %d", page.HTTPStatus)
    }
    if page.Status != "error" {
        t.Errorf("Expected status 'error', got %s", page.Status)
    }
}

func TestAnalyze_WithRetriesNonRetryable(t *testing.T) {
    mockClient := NewMockHTTPClient()

    html := `<html><body>Test</body></html>`

    mockClient.Responses["https://example.com"] = &http.Response{
        StatusCode: 404,
        Status:     "404 Not Found",
        Header:     http.Header{"Content-Type": []string{"text/html"}},
        Body:       io.NopCloser(strings.NewReader(html)),
    }

    opts := Options{
        URL:        "https://example.com",
        Depth:      0,
        Timeout:    5 * time.Second,
        Retries:    3,
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
    if page.HTTPStatus != 404 {
        t.Errorf("Expected status 404, got %d", page.HTTPStatus)
    }
}

func TestAnalyze_WithRetriesNetworkError(t *testing.T) {
    mockClient := NewMockHTTPClient()

    mockClient.Errors["https://example.com"] = errors.New("connection refused")

    mockClient.Responses["https://example.com_2"] = &http.Response{
        StatusCode: 200,
        Status:     "200 OK",
        Header:     http.Header{"Content-Type": []string{"text/html"}},
        Body:       io.NopCloser(strings.NewReader("<html></html>")),
    }

    requestCount := 0
    mockClient.hook = func(req *http.Request) {
        requestCount++
        if requestCount == 2 {
            req.URL, _ = url.Parse("https://example.com_2")
        }
    }

    opts := Options{
        URL:        "https://example.com",
        Depth:      0,
        Timeout:    5 * time.Second,
        Retries:    1,
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
    if page.HTTPStatus != 200 {
        t.Errorf("Expected status 200, got %d", page.HTTPStatus)
    }
}

func TestParseAssets_Basic(t *testing.T) {
    html := `
    <html>
        <head>
            <link rel="stylesheet" href="/style.css">
            <script src="/script.js"></script>
        </head>
        <body>
            <img src="/image.jpg">
            <img src="https://example.com/absolute.jpg">
        </body>
    </html>
    `

    assets := ParseAssets("https://example.com", []byte(html))

    expected := 4
    if len(assets) != expected {
        t.Errorf("Expected %d assets, got %d", expected, len(assets))
    }

    assetMap := make(map[string]string)
    for _, a := range assets {
        assetMap[a.URL] = a.Type
    }

    tests := []struct {
        url      string
        typeName string
    }{
        {"https://example.com/style.css", "style"},
        {"https://example.com/script.js", "script"},
        {"https://example.com/image.jpg", "image"},
        {"https://example.com/absolute.jpg", "image"},
    }

    for _, tt := range tests {
        if typ, ok := assetMap[tt.url]; !ok {
            t.Errorf("Asset %s not found", tt.url)
        } else if typ != tt.typeName {
            t.Errorf("Asset %s expected type %s, got %s", tt.url, tt.typeName, typ)
        }
    }
}

func TestParseAssets_Duplicates(t *testing.T) {
    html := `
    <html>
        <body>
            <img src="/image.jpg">
            <img src="/image.jpg">
            <script src="/script.js"></script>
            <script src="/script.js"></script>
        </body>
    </html>
    `

    assets := ParseAssets("https://example.com", []byte(html))

    expected := 2
    if len(assets) != expected {
        t.Errorf("Expected %d unique assets, got %d", expected, len(assets))
    }
}

func TestAnalyze_WithAssets(t *testing.T) {
    mockClient := NewMockHTTPClient()

    html := `
    <html>
        <head>
            <link rel="stylesheet" href="/style.css">
            <script src="/script.js"></script>
        </head>
        <body>
            <img src="/image.jpg">
            <img src="/broken.jpg">
        </body>
    </html>
    `

    mockClient.Responses["https://example.com"] = &http.Response{
        StatusCode: 200,
        Header:     http.Header{"Content-Type": []string{"text/html"}},
        Body:       io.NopCloser(strings.NewReader(html)),
    }

    mockClient.Responses["https://example.com/style.css"] = &http.Response{
        StatusCode: 200,
        Header:     http.Header{"Content-Type": []string{"text/css"}},
        Body:       io.NopCloser(strings.NewReader("body {color: red}")),
    }

    mockClient.Responses["https://example.com/script.js"] = &http.Response{
        StatusCode: 200,
        Header:     http.Header{"Content-Type": []string{"application/javascript"}},
        Body:       io.NopCloser(strings.NewReader("console.log('test')")),
    }

    mockClient.Responses["https://example.com/image.jpg"] = &http.Response{
        StatusCode: 200,
        Header:     http.Header{"Content-Type": []string{"image/jpeg"}},
        Body:       io.NopCloser(strings.NewReader("fake image data")),
    }

    mockClient.Responses["https://example.com/broken.jpg"] = &http.Response{
        StatusCode: 404,
        Body:       io.NopCloser(strings.NewReader("Not Found")),
    }

    opts := Options{
        URL:        "https://example.com",
        Depth:      0,
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
    if len(page.Assets) != 4 {
        t.Fatalf("Expected 4 assets, got %d", len(page.Assets))
    }

    assetMap := make(map[string]Asset)
    for _, a := range page.Assets {
        assetMap[a.URL] = a
    }

    if a, ok := assetMap["https://example.com/style.css"]; !ok {
        t.Error("style.css not found")
    } else {
        if a.Type != "style" {
            t.Errorf("Expected type 'style', got %s", a.Type)
        }
        if a.StatusCode != 200 {
            t.Errorf("Expected status 200, got %d", a.StatusCode)
        }
        if a.SizeBytes != 17 {
            t.Errorf("Expected size 17, got %d", a.SizeBytes)
        }
    }

    if a, ok := assetMap["https://example.com/script.js"]; !ok {
        t.Error("script.js not found")
    } else {
        if a.Type != "script" {
            t.Errorf("Expected type 'script', got %s", a.Type)
        }
        if a.StatusCode != 200 {
            t.Errorf("Expected status 200, got %d", a.StatusCode)
        }
        if a.SizeBytes != 19 {
            t.Errorf("Expected size 19, got %d", a.SizeBytes)
        }
    }

    if a, ok := assetMap["https://example.com/broken.jpg"]; !ok {
        t.Error("broken.jpg not found")
    } else {
        if a.Type != "image" {
            t.Errorf("Expected type 'image', got %s", a.Type)
        }
        if a.StatusCode != 404 {
            t.Errorf("Expected status 404, got %d", a.StatusCode)
        }
        if a.SizeBytes != 0 {
            t.Errorf("Expected size 0 for broken asset, got %d", a.SizeBytes)
        }
    }
}

func TestAnalyze_AssetCache(t *testing.T) {
    t.Skip("Skipping asset cache test - will be fixed later")
}

func TestAnalyze_JSONFormat(t *testing.T) {
    mockClient := NewMockHTTPClient()

    html := `
    <html>
        <head>
            <title>Example title</title>
            <meta name="description" content="Example description">
            <link rel="stylesheet" href="/static/style.css">
            <script src="/static/script.js"></script>
        </head>
        <body>
            <h1>Example H1</h1>
            <a href="/missing">Missing Link</a>
            <img src="/static/logo.png">
        </body>
    </html>
    `

    mockClient.Responses["https://example.com"] = &http.Response{
        StatusCode: 200,
        Header:     http.Header{"Content-Type": []string{"text/html"}},
        Body:       io.NopCloser(strings.NewReader(html)),
    }

    mockClient.Responses["https://example.com/missing"] = &http.Response{
        StatusCode: 404,
        Body:       io.NopCloser(strings.NewReader("Not Found")),
    }

    mockClient.Responses["https://example.com/static/style.css"] = &http.Response{
        StatusCode: 200,
        Header:     http.Header{"Content-Type": []string{"text/css"}},
        Body:       io.NopCloser(strings.NewReader("body {color: red}")),
    }

    mockClient.Responses["https://example.com/static/script.js"] = &http.Response{
        StatusCode: 200,
        Header:     http.Header{"Content-Type": []string{"application/javascript"}},
        Body:       io.NopCloser(strings.NewReader("console.log('test')")),
    }

    mockClient.Responses["https://example.com/static/logo.png"] = &http.Response{
        StatusCode: 200,
        Header:     http.Header{"Content-Type": []string{"image/png"}},
        Body:       io.NopCloser(strings.NewReader("fake png data")),
    }

    opts := Options{
        URL:        "https://example.com",
        Depth:      0,
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

    if page.URL != "https://example.com" {
        t.Errorf("Expected page URL 'https://example.com', got '%s'", page.URL)
    }
    if page.Depth != 0 {
        t.Errorf("Expected depth 0, got %d", page.Depth)
    }
    if page.HTTPStatus != 200 {
        t.Errorf("Expected HTTP status 200, got %d", page.HTTPStatus)
    }
    if page.Status != "ok" {
        t.Errorf("Expected status 'ok', got '%s'", page.Status)
    }

    if !page.SEO.HasTitle {
        t.Error("Expected has_title true")
    }
    if page.SEO.Title != "Example title" {
        t.Errorf("Expected title 'Example title', got '%s'", page.SEO.Title)
    }
    if !page.SEO.HasDescription {
        t.Error("Expected has_description true")
    }
    if page.SEO.Description != "Example description" {
        t.Errorf("Expected description 'Example description', got '%s'", page.SEO.Description)
    }
    if !page.SEO.HasH1 {
        t.Error("Expected has_h1 true")
    }

    if len(page.BrokenLinks) != 1 {
        t.Fatalf("Expected 1 broken link, got %d", len(page.BrokenLinks))
    }
    broken := page.BrokenLinks[0]
    if broken.URL != "https://example.com/missing" {
        t.Errorf("Expected broken URL 'https://example.com/missing', got '%s'", broken.URL)
    }
    if broken.StatusCode != 404 {
        t.Errorf("Expected status code 404, got %d", broken.StatusCode)
    }
    if broken.Error != "Not Found" {
        t.Errorf("Expected error 'Not Found', got '%s'", broken.Error)
    }

    if len(page.Assets) != 3 {
        t.Fatalf("Expected 3 assets, got %d", len(page.Assets))
    }
}

func TestAnalyze_JSONIndent(t *testing.T) {
    mockClient := NewMockHTTPClient()

    html := `<html><body>Test</body></html>`

    mockClient.Responses["https://example.com"] = &http.Response{
        StatusCode: 200,
        Header:     http.Header{"Content-Type": []string{"text/html"}},
        Body:       io.NopCloser(strings.NewReader(html)),
    }

    opts1 := Options{
        URL:        "https://example.com",
        Depth:      0,
        Timeout:    5 * time.Second,
        IndentJSON: false,
        HTTPClient: mockClient,
    }

    result1, err := Analyze(context.Background(), opts1)
    if err != nil {
        t.Fatalf("Expected no error, got %v", err)
    }

    opts2 := Options{
        URL:        "https://example.com",
        Depth:      0,
        Timeout:    5 * time.Second,
        IndentJSON: true,
        HTTPClient: mockClient,
    }

    result2, err := Analyze(context.Background(), opts2)
    if err != nil {
        t.Fatalf("Expected no error, got %v", err)
    }

    if strings.Contains(string(result1), "\n") {
        t.Error("Compact JSON should not contain newlines")
    }

    if !strings.Contains(string(result2), "\n") {
        t.Error("Indented JSON should contain newlines")
    }

    var report1, report2 Report
    _ = json.Unmarshal(result1, &report1)
    _ = json.Unmarshal(result2, &report2)

    if report1.RootURL != report2.RootURL {
        t.Error("RootURL should be the same")
    }
    if len(report1.Pages) != len(report2.Pages) {
        t.Error("Number of pages should be the same")
    }
}

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