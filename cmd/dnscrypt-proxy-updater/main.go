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
	fs := flag.NewFlagSet("dnscrypt-proxy-updater", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	checkOnce := fs.Bool("check-once", false, "poll once, print status, exit (no tray)")
	jsonOut := fs.Bool("json", false, "with -check-once, print JSON")
	quiet := fs.Bool("quiet", false, "log to the config-dir log file only (no stderr)")
	showVer := fs.Bool("version", false, "print dnscrypt-proxy-updater version and exit")
	cfgPath := fs.String("config", "", "path to config.json (default: OS user config dir)")
	current := fs.String("current-version", "", "manual local version override for this run")
	binPath := fs.String("binary-path", "", "dnscrypt-proxy executable to query for this run")
	doNotify := fs.Bool("notify", false, "with -check-once, also show a desktop notification")
	noNotify := fs.Bool("no-notify", false, "disable desktop notifications")
	doInstall := fs.Bool("install", false, "download the official signed archive, verify minisign, and install dnscrypt-proxy for this system")
	noDNS := fs.Bool("no-dns", false, "with -install, do not change system DNS")
	noService := fs.Bool("no-service", false, "with -install, copy files only (do not install/start the service)")
	installDir := fs.String("install-dir", "", "with -install, destination directory")
	doConfigure := fs.Bool("configure", false, "open the local dnscrypt-proxy settings UI (loopback HTTP)")
	doApplyConfig := fs.Bool("apply-config", false, "commit a staged settings directory (used after UAC)")
	doApplyPending := fs.Bool("apply-pending", false, "commit queued settings from the user config pending folder")
	staging := fs.String("staging", "", "with -apply-config, directory of patched toml/list files")

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
		Install:        *doInstall,
		NoDNS:          *noDNS,
		NoService:      *noService,
		InstallDir:     *installDir,
		Configure:      *doConfigure,
		ApplyConfig:    *doApplyConfig,
		ApplyPending:   *doApplyPending,
		Staging:        *staging,
	}, log)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if *doApplyConfig {
		res, err := rt.ApplyStaged(ctx, *staging)
		if err != nil {
			if !*quiet {
				fmt.Fprintln(os.Stderr, err)
			}
			return 1
		}
		if !*quiet {
			fmt.Fprintln(os.Stdout, res.Message)
		}
		return 0
	}

	if *doApplyPending {
		res, err := rt.ApplyPending(ctx)
		if err != nil {
			if !*quiet {
				fmt.Fprintln(os.Stderr, err)
			}
			return 1
		}
		if !*quiet {
			fmt.Fprintln(os.Stdout, res.Message)
		}
		return 0
	}

	if *doConfigure {
		if err := rt.RunSettingsUI(ctx); err != nil {
			if !*quiet {
				fmt.Fprintln(os.Stderr, err)
			}
			return 1
		}
		return 0
	}

	if *doInstall {
		res, err := rt.Install(ctx)
		if err != nil {
			if !*quiet {
				fmt.Fprintln(os.Stderr, err)
			}
			return 1
		}
		if !*quiet {
			fmt.Fprintln(os.Stdout, res.Message)
			if res.BinaryPath != "" {
				fmt.Fprintf(os.Stdout, "binary: %s\n", res.BinaryPath)
			}
		}
		return 0
	}

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
