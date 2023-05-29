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
	"math/big"
	"os"
	"path/filepath"

	"github.com/moby/moby/client"
	"github.com/otiai10/copy"
	log "github.com/sirupsen/logrus"
	"golang.org/x/exp/slices"
)

// App represents an IE App (project) to be packaged.
type App struct {
	sourcePath string
	tmpDir     string
	repo       string
	project    *ComposerProject
}

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
	log.Info(fmt.Sprintf("🏗  creating temporary project copy in %q", tmpDir))
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
	log.Info(fmt.Sprintf("🫙  app repository detected as %q", repo))

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
		log.Info(fmt.Sprintf("🧹  removed temporary folder %q", a.tmpDir))
		a.tmpDir = ""
	}
}

// SetDetails sets the semver (“versionNumber”, oh well) of this release and its
// notes (if any). This automatically sets the versionId to some suitable value
// behind the scenes. At least we think that it might be a suitable versionId
// value.
func (a *App) SetDetails(semver string, releasenotes string) error {
	return setDetails(
		filepath.Join(a.tmpDir, "detail.json"),
		a.repo,
		semver, releasenotes)
}

func setDetails(path string, repo string, semver string, releasenotes string) error {
	detailJSON, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("cannot read detail.json, reason: %w", err)

	}
	var details map[string]any
	err = json.Unmarshal(detailJSON, &details)
	if err != nil {
		return fmt.Errorf("malformed detail.json, reason: %w", err)
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
	log.Info(fmt.Sprintf("📛  semver: %q -> app ID: %q", semver, versionId))

	details["versionNumber"] = semver
	details["versionId"] = versionId

	details["releaseNotes"] = releasenotes

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
func (a *App) PullAndWriteCompose(ctx context.Context, dockerHost string) error {
	log.Info("🚚  pulling images and writing composer project...")
	opts := []client.Opt{
		client.WithAPIVersionNegotiation(),
	}
	if dockerHost != "" {
		opts = append(opts, client.WithHost(dockerHost))
	} else {
		opts = append(opts, client.WithHostFromEnv())
	}
	moby, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return fmt.Errorf("cannot contact Docker daemon, reason: %w", err)
	}
	serviceImages, err := a.project.Images()
	if err != nil {
		return err
	}
	err = a.project.PullImages(
		ctx, moby,
		serviceImages,
		filepath.Join(a.tmpDir, a.repo),
	)
	if err != nil {
		return err
	}
	composerf, err := os.Create(filepath.Join(a.tmpDir, a.repo, "docker-compose.yml"))
	if err != nil {
		return fmt.Errorf("cannot create Docker compose project file, reason: %w", err)
	}
	defer composerf.Close()
	err = a.project.Save(composerf)
	if err != nil {
		return fmt.Errorf("cannot write Docker compose project file, reason: %w", err)
	}
	return nil
}

// Package (finally) packages the IE app project in a IE app package tar file
// indicated by “out”.
func (a *App) Package(out string) error {
	log.Info("🌯  wrapping up...")
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
		log.Info(fmt.Sprintf("   📦  packaging %s", path))
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
	log.Info(fmt.Sprintf("✅  ...IE app package %q successfully created", out))
	return nil // done and dusted.
}
