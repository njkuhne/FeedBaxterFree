// Command feedbaxter is the FeedBaxter server and admin CLI.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/njkuhne/feedbaxter"
)

const usage = `Usage: feedbaxter <command> [flags]

Commands:
  serve [--mode=ingest|worker|api|all]   run the server (default: MODE env, then all)
  migrate                                apply database migrations
  reset-admin [--email=<admin email>]    set a new temporary admin password and print it
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
