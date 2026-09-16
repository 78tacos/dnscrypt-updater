package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"

	"github.com/78tacos/dnscrypt-updater/internal/app"
	"github.com/78tacos/dnscrypt-updater/internal/config"
	"github.com/78tacos/dnscrypt-updater/internal/githubrel"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet("dnscrypt-updater", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	checkOnce := fs.Bool("check-once", false, "poll once, print status, exit (no tray)")
	jsonOut := fs.Bool("json", false, "with -check-once, print JSON")
	quiet := fs.Bool("quiet", false, "log to the config-dir log file only (no stderr)")
	showVer := fs.Bool("version", false, "print dnscrypt-updater version and exit")
	cfgPath := fs.String("config", "", "path to config.json (default: OS user config dir)")
	current := fs.String("current-version", "", "manual local version override for this run")
	binPath := fs.String("binary-path", "", "dnscrypt-proxy executable to query for this run")
	doNotify := fs.Bool("notify", false, "with -check-once, also show a desktop notification")
	noNotify := fs.Bool("no-notify", false, "disable desktop notifications")

	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *showVer {
		fmt.Printf("%s %s\n", app.AppName, app.AppVersion)
		fmt.Printf("watches %s\n", githubrel.LatestURL)
		return 0
	}

	paths, err := config.ResolvePaths(*cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	log, closeLog, err := app.SetupLogger(paths, *quiet)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer closeLog()

	rt, err := app.NewRuntime(app.Options{
		ConfigPath:     *cfgPath,
		CheckOnce:      *checkOnce,
		JSON:           *jsonOut,
		Quiet:          *quiet,
		CurrentVersion: *current,
		BinaryPath:     *binPath,
		ForceNotify:    *doNotify,
		NoNotify:       *noNotify,
	}, log)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if *checkOnce {
		code, err := rt.CheckOnce(ctx, os.Stdout)
		if err != nil && !*jsonOut {
			fmt.Fprintln(os.Stderr, err)
		}
		return code
	}

	log.Info("starting tray companion", "version", app.AppVersion, "config", rt.Paths.File, "upstream", githubrel.LatestURL)
	err = app.RunTray(rt)
	if errors.Is(err, app.ErrTrayUnavailable) {
		log.Info("tray unavailable in this build; running one-shot check (Linux/macOS tray needs: CGO_ENABLED=1 go build -tags systray)")
		code, err := rt.CheckOnce(ctx, os.Stdout)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
		return code
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
