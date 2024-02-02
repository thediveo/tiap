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

	"github.com/google/go-containerregistry/pkg/v1/daemon"
	"golang.org/x/exp/slices"

	log "github.com/sirupsen/logrus"
)

// App represents an IE App (project) to be packaged.
type App struct {
	sourcePath string
	repo       string
	project    *ComposerProject
}

// DefaultIEAppArch is the denormalized platform architecture name of the
// default "unnamed" architecture.
const DefaultIEAppArch = "x86-64"

// NewApp returns an IE App object initialized from the specified “template”
// path.
func NewApp(source string) (a *App, err error) {
	// Copy the "template" app file/folder structure into a temporary place, but
	// skip any Docker composer file for now. However, the remember its
	// containing directory as the "repository".
	log.Info(fmt.Sprintf("🏗  determining repository"))
	repo := ""
	err = filepath.WalkDir(source, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !slices.Contains(composerFiles, d.Name()) {
			return nil
		}
		if repo != "" {
			return errors.New("multiple Docker compose project files")
		}
		repo = filepath.Dir(path)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("cannot scan app template structure, reason: %w", err)
	}
	if repo == "" {
		return nil, errors.New("project lacks Docker compose project file")
	}
	relrepo, err := filepath.Rel(source, repo)
	if err != nil {
		return nil, errors.New("cannot determine relative repository path")
	}
	log.Info(fmt.Sprintf("🫙  app repository detected as %q", relrepo))

	// Try to locate and load the Docker composer project
	//
	project, err := LoadComposerProject(filepath.Join(source, repo))
	if err != nil {
		return nil, err
	}

	a = &App{
		sourcePath: source,
		repo:       repo,
		project:    project,
	}
	return
}

// SetDetails sets the semver (“versionNumber”, oh well) of this release, notes
// (if any) and optional architecture, and then writes a new “detail.json” into
// the specified tar writer.
//
// Note: SetDetails automatically sets the versionId to some suitable value
// behind the scenes. At least we think that it might be a suitable versionId
// value.
func (a *App) SetDetails(
	tarw *tar.Writer,
	semver string,
	releasenotes string,
	iearch string,
) error {
	return writeDetails(
		tarw,
		filepath.Join(a.sourcePath, "detail.json"),
		a.repo,
		semver, releasenotes, iearch)
}

func writeDetails(
	tarw *tar.Writer,
	path string,
	repo string,
	semver string,
	releasenotes string,
	iearch string,
) error {
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

	// set the IE App architecture only if it isn't empty and it's not the
	// default (x86-64) architecture.
	if iearch != "" && iearch != DefaultIEAppArch {
		details["arch"] = iearch
	}

	detailJSON, err = json.Marshal(details)
	if err != nil {
		return fmt.Errorf("cannot JSONize detail information, reason: %w", err)
	}
	err = tarw.WriteHeader(&tar.Header{
		Typeflag:   tar.TypeReg,
		Name:       filepath.ToSlash("detail.json"),
		Mode:       0644,
		ModTime:    defaultMtime.UTC(),
		AccessTime: defaultMtime.UTC(),
		ChangeTime: defaultMtime.UTC(),
		Size:       int64(len(detailJSON)),
		Uid:        defaultAppUID,
		Gid:        defaultAppGID,
	})
	if err == nil {
		_, err = tarw.Write(detailJSON)
	}
	if err != nil {
		return fmt.Errorf("cannot write detail.json, reason: %w", err)
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
	log.Info("🚚  pulling images and writing composer project...")
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
	err = nil // FIXME: WriteDigests(digestJson, a.tmpDir)
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
