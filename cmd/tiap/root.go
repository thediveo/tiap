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
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/containerd/containerd/platforms"
	"github.com/moby/moby/client"
	ispecsv1 "github.com/opencontainers/image-spec/specs-go/v1"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/thediveo/tiap"
	"golang.org/x/exp/slices"
	"golang.org/x/sys/unix"
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

func unerringly[R any](r R, err error) R {
	if err != nil {
		log.Fatal(err.Error())
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

			rn := strings.Replace(
				successfully(rootCmd.Flags().GetString(releaseNotesFlag)),
				"\n", "\\n", -1)
			releaseNotes, err := strconv.Unquote(`"` + rn + `"`)
			if err != nil {
				log.Fatalf("release notes %q: %s", successfully(rootCmd.Flags().GetString(releaseNotesFlag)), err.Error())
			}

			app, err := tiap.NewApp(args[0])
			if err != nil {
				return err
			}
			defer app.Done()

			platform := unerringly(
				platforms.Parse(successfully(rootCmd.Flags().GetString(platformFlag))))
			if platform.OS != "linux" && platform.OS != runtime.GOOS {
				// warn when the platform OS was (explicitly) set to something
				// different than linux; we try to not warn in case tiap is run
				// on a different OS and the platform has been specified only
				// regarding its architecture, but not OS and the unwanted
				// default OS has kicked in.
				log.Warnf("enforcing \"linux\" platform OS")
			}
			platform.OS = "linux" // Industrial Edge supports only Linux.
			log.Infof("🚊  normalized platform: %q", platforms.Format(platform))

			appArch := denormalize(platform).Architecture
			log.Infof("🚊  denormalized IE App architecture: %q", appArch)

			err = app.SetDetails(appSemver, releaseNotes, appArch)
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
				platforms.Format(platform),
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
		"release notes (interpreted as double-quoted Go string literal; use \\n, \\\", …)")

	p := thisPlatform()
	rootCmd.Flags().StringP(platformFlag, "p", "linux/"+p.Architecture,
		"platform to build app for")

	rootCmd.Flags().Bool(pullAlwaysFlag, false,
		"always pull image from remote registry, never use local images")

	rootCmd.Flags().StringP(dockerHostFlag, "H", "",
		"Docker daemon socket to connect to (only if non-default and using local images)")

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
