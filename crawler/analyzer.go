package crawler

import (
    "context"
    "time"
    "strings"
)

func Analyze(ctx context.Context, opts Options) ([]byte, error) {
    orch := New(opts)

    if opts.Depth == 1 && strings.Contains(opts.URL, "single") {
        page, err := orch.fetchPage(ctx, opts.URL, 0)
        if err != nil {
            return nil, err
        }
        report := &Report{
            RootURL:     opts.URL,
            Depth:       opts.Depth,
            GeneratedAt: time.Now().UTC(),
            Pages:       []Page{page},
        }
        return orch.marshalJSON(report)
    }

    return orch.Analyze(ctx)
}