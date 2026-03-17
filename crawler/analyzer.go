package crawler

import (
    "context"
)

func Analyze(ctx context.Context, opts Options) ([]byte, error) {
    orch := New(opts)
    return orch.Analyze(ctx)
}