// Copyright 2023 by Harald Albrecht
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may not
// use this file except in compliance with the License. You may obtain a copy
// of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations
// under the License.

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/containerd/platforms"
	"github.com/lmittmann/tint"
	"github.com/moby/moby/client"
	ispecsv1 "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/spf13/cobra"
	"github.com/thediveo/tiap"
	"golang.org/x/exp/slices"
	"golang.org/x/sys/unix"
)

// Names of CLI flags
const (
	outnameFlagName       = "out"
	appVersionFlagName    = "app-version"
	releaseNotesFlagName  = "release-notes"
	platformFlagName      = "platform"
	pullAlwaysFlagName    = "pull-always"
	dockerHostFlagName    = "host"
	interpolationFlagName = "interpolate"
	debugFlagName         = "debug"
)

// successfully expects the returned value-error pair to be without error;
// otherwise, it panics with the passed error. Use this helper in those
// situations where there is a code problem that the user cannot fix (except by
// hacking the source).
func successfully[R any](r R, err error) R {
	if err != nil {
		panic(err)
	}
	return r
}

// unerringly expects the returned value-error pair to be without error;
// otherwise, it logs an error and exits with code 1.
func unerringly[R any](r R, err error) R {
	if err != nil {
		slog.Error("fatal", slog.String("error", err.Error()))
		os.Exit(1)
	}
	return r
}

// thisPlatform returns a platform specification consisting of only the
// architecture of the OS we're currently running on. We don't need the OS as
// Industrial Edge supports Linux only.
func thisPlatform() ispecsv1.Platform {
	var utsname unix.Utsname
	if err := unix.Uname(&utsname); err != nil {
		copy(utsname.Machine[:], []byte(runtime.GOARCH))
	}
	return platforms.Normalize(ispecsv1.Platform{
		Architecture: unix.ByteSliceToString(utsname.Machine[:]),
	})
}

// denormalizes the OCI platform specification architecture into the Industrial
// Edge usage. See
// https://docs.eu1.edge.siemens.cloud/intro/glossary/glossary.html#x86-64 and
// https://docs.eu1.edge.siemens.cloud/intro/glossary/glossary.html#arm64.
func denormalize(p ispecsv1.Platform) ispecsv1.Platform {
	p = platforms.Normalize(p)
	switch p.Architecture {
	case "amd64":
		p.Architecture = tiap.DefaultIEAppArch
	}
	return p
}

// buildInfo returns the value of the specified key into the BuildSettings.
func buildInfo(info *debug.BuildInfo, key string) string {
	idx := slices.IndexFunc(info.Settings,
		func(setting debug.BuildSetting) bool {
			return setting.Key == key
		})
	if idx < 0 {
		return ""
	}
	return info.Settings[idx].Value
}

func newRootCmd() (rootCmd *cobra.Command) {
	rootCmd = &cobra.Command{
		Use:     "tiap -o FILE [flags] APP-TEMPLATE-DIR",
		Short:   "tiap isn't app publisher, but packages Industrial Edge .app files anyway",
		Version: `":latest"`, // sorry :p
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slogOpts := slog.HandlerOptions{
				Level: slog.LevelInfo,
			}
			if successfully(rootCmd.Flags().GetBool(debugFlagName)) {
				slogOpts.Level = slog.LevelDebug
			}
			slog.SetDefault(slog.New(
				tint.NewHandler(os.Stderr, &tint.Options{
					Level:      slogOpts.Level,
					TimeFormat: time.RFC3339,
				}),
			))
			slog.Info("tiap ... isn't app publisher",
				slog.String("version", rootCmd.Version),
				slog.String("license", "Apache 2.0"))
			slog.Debug("debug logging enabled")

			appSemver := successfully(rootCmd.Flags().GetString(appVersionFlagName))
			if appSemver == "" {
				slog.Debug("determining semvar using git")
				out, err := exec.Command("git", "describe").CombinedOutput()
				if err != nil {
					slog.Error("git describe failed", slog.String("output", string(out)))
					return fmt.Errorf("git describe failed: %s", out)
				}
				appSemver = strings.Trim(string(out), "\r\n")
			}
			appSemver = strings.TrimPrefix(appSemver, "v")
			if _, err := semver.StrictNewVersion(appSemver); err != nil {
				return fmt.Errorf("invalid app semver %q, reason: %w",
					appSemver, err)
			}
			slog.Debug("app project", slog.String("semver", appSemver))

			releaseNotes := successfully(rootCmd.Flags().GetString(releaseNotesFlagName))
			rn := strings.Replace(releaseNotes, "\n", "\\n", -1)
			releaseNotes, err := strconv.Unquote(`"` + rn + `"`)
			if err != nil {
				slog.Error("release notes",
					slog.String("contents", releaseNotes),
					slog.String("error", err.Error()))
				os.Exit(1)
			}

			app, err := tiap.NewApp(args[0])
			if err != nil {
				return err
			}
			defer app.Done()

			var vars map[string]string // nil means no interpolation at all
			if successfully(rootCmd.Flags().GetBool(interpolationFlagName)) {
				vars = envVars()
			}
			if vars != nil {
				if err := app.Interpolate(vars); err != nil {
					slog.Error("interpolating compose project variables",
						slog.String("error", err.Error()))
					os.Exit(1)
				}
			}

			platform := unerringly(
				platforms.Parse(successfully(rootCmd.Flags().GetString(platformFlagName))))
			if platform.OS != "linux" && platform.OS != runtime.GOOS {
				// warn when the platform OS was (explicitly) set to something
				// different than linux; we try to not warn in case tiap is run
				// on a different OS and the platform has been specified only
				// regarding its architecture, but not OS and the unwanted
				// default OS has kicked in.
				slog.Warn("enforcing \"linux\" platform OS")
			}
			platform.OS = "linux" // Industrial Edge supports only Linux.
			slog.Info("normalized platform",
				slog.String("platform", platforms.Format(platform)))

			appArch := denormalize(platform).Architecture
			slog.Info("denormalized IE App architecture",
				slog.String("arch", appArch))

			err = app.SetDetails(appSemver, releaseNotes, appArch, envVars())
			if err != nil {
				return err
			}

			pullAlways := successfully(rootCmd.Flags().GetBool(pullAlwaysFlagName))
			var moby *client.Client
			if !pullAlways {
				slog.Debug("creating Docker/Moby client")
				dockerHost := successfully(rootCmd.Flags().GetString(dockerHostFlagName))
				opts := []client.Opt{
					client.WithAPIVersionNegotiation(),
				}
				if dockerHost != "" {
					opts = append(opts, client.WithHost(dockerHost))
				} else {
					opts = append(opts, client.WithHostFromEnv())
				}
				moby, err = client.NewClientWithOpts(opts...)
				if err != nil {
					return fmt.Errorf("cannot contact Docker daemon, reason: %w", err)
				}
				defer moby.Close()
				slog.Debug("Docker/Moby client created")
			}

			err = app.PullAndWriteCompose(
				context.Background(),
				platforms.Format(platform),
				moby)
			if err != nil {
				return err
			}

			outname := successfully(rootCmd.Flags().GetString(outnameFlagName))
			if filepath.Ext(outname) == "" {
				outname = outname + ".app"
			}
			return app.Package(outname)
		},
	}

	flags := rootCmd.Flags()

	flags.StringP(outnameFlagName, "o", "",
		"mandatory: name of app package file to write")
	if err := rootCmd.MarkFlagRequired(outnameFlagName); err != nil {
		panic(err)
	}

	flags.String(appVersionFlagName, "",
		"app semantic version, defaults to git describe")

	flags.String(releaseNotesFlagName, "",
		"release notes (interpreted as double-quoted Go string literal; use \\n, \\\", …)")

	p := thisPlatform()
	flags.StringP(platformFlagName, "p", "linux/"+p.Architecture,
		"platform to build app for")

	flags.Bool(pullAlwaysFlagName, false,
		"always pull image from remote registry, never use local images")

	flags.StringP(dockerHostFlagName, "H", "",
		"Docker daemon socket to connect to (only if non-default and using local images)")

	rootCmd.MarkFlagsMutuallyExclusive(pullAlwaysFlagName, dockerHostFlagName)

	flags.BoolP(interpolationFlagName, "i", false,
		"interpolate env vars in compose project and detail.json")

	flags.Bool(debugFlagName, false,
		"enable debug logging")

	if info, biok := debug.ReadBuildInfo(); biok {
		commit := buildInfo(info, "vcs.revision")
		if commit != "" {
			modified := ""
			if buildInfo(info, "vcs.modified") == "true" {
				modified = " (modified)"
			}
			rootCmd.Version = fmt.Sprintf("commit %s%s", commit[:8], modified)
		} else if modver := info.Main.Version; modver != "" {
			rootCmd.Version = modver
		}
	}

	return rootCmd
}

// envVars returns a map of key-value environment variables.
func envVars() map[string]string {
	envvars := map[string]string{}
	for _, keyval := range os.Environ() {
		fields := strings.SplitN(keyval, "=", 2)
		if len(fields) < 2 {
			continue
		}
		envvars[fields[0]] = fields[1]
	}
	return envvars
}
