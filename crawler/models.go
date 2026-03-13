package crawler

import (
    "net/http"
    "time"
)

type Options struct {
    URL         string
    Depth       int
    Retries     int
    Delay       time.Duration
    Timeout     time.Duration
    UserAgent   string
    Concurrency int
    IndentJSON  bool
    HTTPClient  interface {
        Do(req *http.Request) (*http.Response, error)
    }
}

type Report struct {
    RootURL     string    `json:"root_url"`
    Depth       int       `json:"depth"`
    GeneratedAt time.Time `json:"generated_at"`
    Pages       []Page    `json:"pages"`
}

type Page struct {
    URL          string       `json:"url"`
    Depth        int          `json:"depth"`
    HTTPStatus   int          `json:"http_status"`
    Status       string       `json:"status"`
    Error        string       `json:"error"`
    SEO          SEO          `json:"seo"`
    BrokenLinks  []BrokenLink `json:"broken_links"`
    DiscoveredAt time.Time    `json:"discovered_at"`
}

type SEO struct {
    HasTitle       bool   `json:"has_title"`
    Title          string `json:"title"`
    HasDescription bool   `json:"has_description"`
    Description    string `json:"description"`
    HasH1          bool   `json:"has_h1"`
    H1             string `json:"h1"`
}

type BrokenLink struct {
    URL        string `json:"url"`
    StatusCode int    `json:"status_code,omitempty"`
    Error      string `json:"error,omitempty"`
}