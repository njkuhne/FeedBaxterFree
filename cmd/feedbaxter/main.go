// Command feedbaxter is the FeedBaxter server and admin CLI.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/njkuhne/feedbaxter"
	"github.com/njkuhne/feedbaxter/internal/config"
)

const usage = `Usage: feedbaxter <command> [flags]

Commands:
  serve [--mode=ingest|worker|api|all]   run the server (default: MODE env, then all)
  migrate                                apply database migrations
  reset-admin [--email=<admin email>]    set a new temporary admin password and print it
  healthcheck                            exit 0 if /readyz on HTTP_ADDR answers 200 (for container health checks)
  version                                print the version
  help                                   print this help
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch args[0] {
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return 0
	case "version":
		fmt.Fprintf(stdout, "feedbaxter %s\n", feedbaxter.Version)
		return 0
	case "serve":
		fs := flag.NewFlagSet("serve", flag.ContinueOnError)
		fs.SetOutput(stderr)
		mode := fs.String("mode", "", "ingest, worker, api or all (default: MODE env)")
		if err := fs.Parse(args[1:]); err != nil {
			fmt.Fprint(stderr, usage)
			return 2
		}
		if fs.NArg() > 0 {
			fmt.Fprintf(stderr, "feedbaxter: unexpected argument %q\n%s", fs.Arg(0), usage)
			return 2
		}
		return report(stderr, feedbaxter.Run(ctx, feedbaxter.Options{Mode: *mode}))
	case "healthcheck":
		if len(args) > 1 {
			fmt.Fprintf(stderr, "feedbaxter: unexpected argument %q\n%s", args[1], usage)
			return 2
		}
		cfg, err := config.Load(nil)
		if err != nil {
			return report(stderr, err)
		}
		url, err := readyURL(cfg.HTTPAddr)
		if err != nil {
			return report(stderr, err)
		}
		return report(stderr, checkReady(ctx, url))
	case "migrate":
		applied, err := feedbaxter.Migrate(ctx, feedbaxter.Options{})
		if err != nil {
			return report(stderr, err)
		}
		if len(applied) == 0 {
			fmt.Fprintln(stdout, "schema is up to date")
		}
		for _, v := range applied {
			fmt.Fprintf(stdout, "applied %s\n", v)
		}
		return 0
	case "reset-admin":
		fs := flag.NewFlagSet("reset-admin", flag.ContinueOnError)
		fs.SetOutput(stderr)
		email := fs.String("email", "", "admin to reset (default: the first admin)")
		if err := fs.Parse(args[1:]); err != nil {
			fmt.Fprint(stderr, usage)
			return 2
		}
		if fs.NArg() > 0 {
			fmt.Fprintf(stderr, "feedbaxter: unexpected argument %q\n%s", fs.Arg(0), usage)
			return 2
		}
		got, pw, err := feedbaxter.ResetAdmin(ctx, feedbaxter.Options{}, *email)
		if err != nil {
			return report(stderr, err)
		}
		fmt.Fprintf(stdout, "temporary password for %s: %s\n", got, pw)
		return 0
	default:
		fmt.Fprintf(stderr, "feedbaxter: unknown command %q\n%s", args[0], usage)
		return 2
	}
}

func report(stderr io.Writer, err error) int {
	if err == nil {
		return 0
	}
	fmt.Fprintf(stderr, "feedbaxter: %v\n", err)
	return 1
}

// readyURL builds the /readyz URL for the server listening on httpAddr,
// mapping wildcard hosts to loopback.
func readyURL(httpAddr string) (string, error) {
	host, port, err := net.SplitHostPort(httpAddr)
	if err != nil {
		return "", fmt.Errorf("invalid HTTP_ADDR %q: %w", httpAddr, err)
	}
	switch host {
	case "", "0.0.0.0", "::":
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port) + "/readyz", nil
}

// checkReady returns nil when url answers 200.
func checkReady(ctx context.Context, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("readyz returned %d", resp.StatusCode)
	}
	return nil
}
