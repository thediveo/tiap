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
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/moby/moby/client"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/thediveo/tiap"
	"golang.org/x/exp/slices"
)

const (
	outnameFlag      = "out"
	appVersionFlag   = "app-version"
	releaseNotesFlag = "release-notes"
	platformFlag     = "platform"
	pullAlwaysFlag   = "pull-always"
	dockerHostFlag   = "host"
)

func successfully[R any](r R, err error) R {
	if err != nil {
		panic(err)
	}
	return r
}

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
		Use:     "tiap [flags] [app-template-dir]",
		Short:   "tiap isn't app publisher, but packages Industrial Edge .app files anyway",
		Version: `":latest"`, // sorry :p
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			log.Info("🗩  tiap ... isn't app publisher")
			log.Info(fmt.Sprintf("   %s", rootCmd.Version))
			log.Info("⚖  Apache 2.0 License")

			appSemver := successfully(rootCmd.Flags().GetString(appVersionFlag))
			if appSemver == "" {
				out, err := exec.Command("git", "describe").CombinedOutput()
				if err != nil {
					log.Errorf(fmt.Sprintf("git describe: %s", out))
					return fmt.Errorf("git describe failed: %s", out)
				}
				appSemver = strings.Trim(string(out), "\r\n")
			}
			appSemver = strings.TrimPrefix(appSemver, "v")
			if _, err := semver.StrictNewVersion(appSemver); err != nil {
				return fmt.Errorf("invalid app semver %q, reason: %w",
					appSemver, err)
			}

			releaseNotes := successfully(rootCmd.Flags().GetString(releaseNotesFlag))

			app, err := tiap.NewApp(args[0])
			if err != nil {
				return err
			}
			defer app.Done()

			platform := successfully(rootCmd.Flags().GetString(platformFlag))
			log.Infof("🚊  platform: %q", platform)

			err = app.SetDetails(appSemver, releaseNotes, platform)
			if err != nil {
				return err
			}

			pullAlways := successfully(rootCmd.Flags().GetBool(pullAlwaysFlag))
			var moby *client.Client
			if !pullAlways {
				dockerHost := successfully(rootCmd.Flags().GetString(dockerHostFlag))
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
			}

			err = app.PullAndWriteCompose(
				context.Background(),
				platform,
				moby)
			if err != nil {
				return err
			}

			outname := successfully(rootCmd.Flags().GetString(outnameFlag))
			if filepath.Ext(outname) == "" {
				outname = outname + ".app"
			}
			return app.Package(outname)
		},
	}
	rootCmd.Flags().StringP(outnameFlag, "o", "",
		"mandatory: name of app package file to write")
	if err := rootCmd.MarkFlagRequired(outnameFlag); err != nil {
		panic(err)
	}

	rootCmd.Flags().String(appVersionFlag, "",
		"app semantic version, defaults to git describe")

	rootCmd.Flags().String(releaseNotesFlag, "",
		"release notes")

	rootCmd.Flags().StringP(platformFlag, "p", thisPlatform(),
		"platform to build app for")

	rootCmd.Flags().Bool(pullAlwaysFlag, false,
		"always pull image from remote registry, never use local images")

	rootCmd.Flags().StringP(dockerHostFlag, "H", "",
		"Docker daemon socket to connect to")

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
