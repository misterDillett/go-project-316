package testutil

import (
    "io"
    "net/http"
    "strings"
)

type MockHTTPClient struct {
    Responses map[string]*http.Response
    Errors    map[string]error
    DefaultResponse *http.Response
    DefaultError    error
    Hook      func(*http.Request)
}

func NewMockHTTPClient() *MockHTTPClient {
    return &MockHTTPClient{
        Responses: make(map[string]*http.Response),
        Errors:    make(map[string]error),
    }
}

func (m *MockHTTPClient) Do(req *http.Request) (*http.Response, error) {
    if m.Hook != nil {
        m.Hook(req)
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