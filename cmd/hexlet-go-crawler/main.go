package main

import (
    "context"
    "fmt"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"

    "code/crawler"
    "github.com/urfave/cli/v2"
)

func main() {
    app := &cli.App{
        Name:    "hexlet-go-crawler",
        Usage:   "analyze a website structure",
        Version: "1.0.0",
        Flags: []cli.Flag{
            &cli.IntFlag{
                Name:  "depth",
                Value: 10,
                Usage: "crawl depth",
            },
            &cli.IntFlag{
                Name:  "retries",
                Value: 1,
                Usage: "number of retries for failed requests",
            },
            &cli.DurationFlag{
                Name:  "delay",
                Value: 0,
                Usage: "delay between requests (example: 200ms, 1s)",
            },
            &cli.DurationFlag{
                Name:  "timeout",
                Value: 15 * time.Second,
                Usage: "per-request timeout",
            },
            &cli.StringFlag{
                Name:  "user-agent",
                Value: "HexletCrawler/1.0",
                Usage: "custom user agent",
            },
            &cli.IntFlag{
                Name:  "workers",
                Value: 4,
                Usage: "number of concurrent workers",
            },
        },
        Action: func(c *cli.Context) error {
            if c.NArg() == 0 {
                return fmt.Errorf("URL is required")
            }

            url := c.Args().First()

            ctx, cancel := context.WithCancel(context.Background())
            defer cancel()

            sigCh := make(chan os.Signal, 1)
            signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
            go func() {
                <-sigCh
                cancel()
            }()

            httpClient := &http.Client{
                Timeout: c.Duration("timeout"),
            }

            opts := crawler.Options{
                URL:         url,
                Depth:       c.Int("depth"),
                Retries:     c.Int("retries"),
                Delay:       c.Duration("delay"),
                Timeout:     c.Duration("timeout"),
                UserAgent:   c.String("user-agent"),
                Concurrency: c.Int("workers"),
                IndentJSON:  true,
                HTTPClient:  httpClient,
            }

            orchestrator := crawler.New(opts)
            result, err := orchestrator.Analyze(ctx)
            if err != nil {
                return err
            }

            fmt.Println(string(result))
            return nil
        },
    }

    if err := app.Run(os.Args); err != nil {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        os.Exit(1)
    }
}