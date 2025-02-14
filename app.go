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

package tiap

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"time"

	"github.com/google/go-containerregistry/pkg/v1/daemon"
	"github.com/otiai10/copy"
	"github.com/thediveo/tiap/interpolate"
	"golang.org/x/exp/slices"
)

// App represents an IE App (project) to be packaged.
type App struct {
	sourcePath string
	tmpDir     string
	repo       string
	project    *ComposerProject
}

// DefaultIEAppArch is the denormalized platform architecture name of the
// default "unnamed" architecture.
const DefaultIEAppArch = "x86-64"

// NewApp returns an IE App object initialized from the specified “template”
// path.
func NewApp(source string) (a *App, err error) {
	tmpDir, err := os.MkdirTemp("", "tiap-project-*")
	if err != nil {
		return nil, fmt.Errorf("cannot create temporary project directory, reason: %w", err)
	}
	defer func() {
		if err != nil && tmpDir != "" {
			os.RemoveAll(tmpDir)
			a = nil
		}
	}()

	// Copy the "template" app file/folder structure into a temporary place, but
	// skip any Docker composer file for now. However, the notice its directory
	// as the "repository".
	slog.Info("creating temporary project copy",
		slog.String("path", tmpDir))
	repo := ""
	err = copy.Copy(source, tmpDir, copy.Options{
		Skip: func(info os.FileInfo, src, dest string) (bool, error) {
			if slices.Contains(composerFiles, info.Name()) {
				repo = filepath.Dir(src)
				return true, nil
			}
			return false, nil
		},
	})
	if err != nil {
		return nil, fmt.Errorf("cannot copy app template structure, reason: %w", err)
	}
	if repo == "" {
		return nil, errors.New("project lacks Docker compose project file")
	}
	repo, err = filepath.Rel(source, repo)
	if err != nil {
		return nil, errors.New("cannot determine relative repository path")
	}
	slog.Info("app repository detected",
		slog.String("repo", repo))

	// Try to locate and load the Docker composer project
	//
	project, err := LoadComposerProject(filepath.Join(source, repo))
	if err != nil {
		return nil, err
	}

	a = &App{
		sourcePath: source,
		tmpDir:     tmpDir,
		repo:       repo,
		project:    project,
	}
	return
}

// Done removes all temporary work files.
func (a *App) Done() {
	if a.tmpDir != "" {
		os.RemoveAll(a.tmpDir)
		slog.Info("removed temporary folder",
			slog.String("path", a.tmpDir))
		a.tmpDir = ""
	}
}

// Interpolate all variables in the app's composer project using the specified
// variables, updating the project's YAML data accordingly. In case of
// interpolation problems, it returns an error, otherwise nil.
func (a *App) Interpolate(vars map[string]string) error {
	return a.project.Interpolate(vars)
}

// SetDetails sets the semver (“versionNumber”, oh well) of this release, notes
// (if any) and optional architecture, and then writes a new “detail.json”
// into the build directory. This automatically sets the versionId to some
// suitable value behind the scenes. At least we think that it might be a
// suitable versionId value.
func (a *App) SetDetails(semver string, releasenotes string, iearch string, vars map[string]string) error {
	return setDetails(
		filepath.Join(a.tmpDir, "detail.json"),
		a.repo,
		semver, releasenotes, iearch,
		vars)
}

func setDetails(
	path string,
	repo string,
	semver string,
	releasenotes string,
	iearch string,
	vars map[string]string,
) error {
	// First read in and parse the detail.json file, before working on the, erm,
	// details.
	detailJSON, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("cannot read detail.json, reason: %w", err)

	}
	var details map[string]any
	err = json.Unmarshal(detailJSON, &details)
	if err != nil {
		return fmt.Errorf("malformed detail.json, reason: %w", err)
	}

	// If interpolation has been enabled, interpolate all variables that might
	// be lurking in the string elements of the JSON data, either in plain
	// strings, object field values, or inside arrays of string values.
	if vars != nil {
		slog.Debug("interpolating detail.json environment variables")
		interpolDetails, err := interpolate.Variables(details, vars)
		if err != nil {
			return fmt.Errorf("malformed detail.json, reason: %w", err)
		}
		details = interpolDetails // *scnr*
	}

	// dunno what versionId encodes, it seems to suffice that it is just a
	// unique string of 32 characters in the 0-9, a-z, A-Z set. It doesn't seem
	// to be base64 so base62 could be a good bet. We simply hash the semver
	// string (even if its low entropy) and the repo dir name.
	digester := sha256.New()
	digester.Write([]byte(semver))
	digester.Write([]byte(repo))
	// Thanks to https://ucarion.com/go-base62 for the stdlib (mis)use as a
	// stock base62 encoder ;)
	var bi big.Int
	bi.SetBytes(digester.Sum(nil))
	versionId := bi.Text(62)[:32]
	slog.Info("updated version ID based on semver",
		slog.String("semver", semver),
		slog.String("versionId", versionId))

	details["versionNumber"] = semver
	details["versionId"] = versionId

	if releasenotes != "" {
		details["releaseNotes"] = releasenotes
	}

	// set the IE App architecture only if it isn't empty and it's not the
	// default (x86-64) architecture.
	if iearch != "" && iearch != DefaultIEAppArch {
		details["arch"] = iearch
	}

	detailJSON, err = json.Marshal(details)
	if err != nil {
		return fmt.Errorf("cannot JSONize detail information, reason: %w", err)
	}
	err = os.WriteFile(path, detailJSON, 0666)
	if err != nil {
		return fmt.Errorf("cannot update detail.json, reason: %w", err)
	}
	return nil
}

// PullAndWriteCompose analyzes the project's compose deployment in order to
// pull the required container images, then saves the images into the temporary
// stage, and writes composer project.
func (a *App) PullAndWriteCompose(
	ctx context.Context,
	platform string,
	optclient daemon.Client,
) error {
	slog.Info("pulling images...")
	serviceImages, err := a.project.Images()
	if err != nil {
		return err
	}
	err = a.project.PullImages(
		ctx,
		serviceImages,
		platform,
		filepath.Join(a.tmpDir, a.repo),
		optclient,
	)
	if err != nil {
		return err
	}
	slog.Info("images successfully pulled")
	slog.Info("writing final compose project...")
	composerf, err := os.Create(filepath.Join(a.tmpDir, a.repo, "docker-compose.yml"))
	if err != nil {
		return fmt.Errorf("cannot create Docker compose project file, reason: %w", err)
	}
	defer composerf.Close()
	err = a.project.Save(composerf)
	if err != nil {
		return fmt.Errorf("cannot write Docker compose project file, reason: %w", err)
	}
	slog.Info("final compose project written")
	return nil
}

// Package (finally) packages the IE app project in a IE app package tar file
// indicated by “out”.
func (a *App) Package(out string) error {
	slog.Info("wrapping up...")
	start := time.Now()
	defer func() {
		duration := time.Duration(math.Ceil(time.Since(start).Seconds())) * time.Second
		slog.Info("IE app package",
			slog.String("path", out),
			slog.Duration("duration", duration))
	}()
	// Calculate and write digests
	digestJson, err := os.Create(filepath.Join(a.tmpDir, "digests.json"))
	if err != nil {
		return fmt.Errorf("cannot create digests.json, reason: %w", err)
	}
	err = WriteDigests(digestJson, a.tmpDir)
	digestJson.Close()
	if err != nil {
		return err
	}

	// Doctor Tarr and Professor Fether
	slog.Info("creating IE app tar-ball",
		slog.String("doctor", "Tarr"),
		slog.String("professor", "Fether"))
	tarball, err := os.Create(out)
	if err != nil {
		return fmt.Errorf("cannot create IE app package file, reason: %w", err)
	}
	defer tarball.Close()
	tarrer := tar.NewWriter(tarball)
	defer tarrer.Close()
	rootfs := os.DirFS(a.tmpDir)
	err = fs.WalkDir(rootfs, ".", func(path string, dirEntry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == "." {
			return nil
		}
		slog.Info("packaging", slog.String("path", path))
		stat, err := fs.Stat(rootfs, path)
		if err != nil {
			return err
		}
		header, err := tar.FileInfoHeader(stat, path)
		if err != nil {
			return err
		}
		header.Uid = 1000
		header.Gid = 1000
		header.Name = filepath.ToSlash(path)
		err = tarrer.WriteHeader(header)
		if err != nil {
			return err
		}
		if dirEntry.IsDir() {
			return nil
		}
		// Only copy contents if it's a regular file.
		file, err := rootfs.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(tarrer, file)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("cannot package IE app, reason: %w", err)
	}
	slog.Info("IE app package successfully created")
	return nil // done and dusted.
}
