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
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"math"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/distribution/reference"
	"github.com/google/go-containerregistry/pkg/v1/daemon"
	"github.com/thediveo/nonstd/sets"
	"gopkg.in/yaml.v3"
)

// https://docs.docker.com/compose/compose-file/03-compose-file/ says that
// “.yaml” is preferred over “.yml”.
var composerFiles = []string{
	"docker-compose.yaml",
	"docker-compose.yml",
}

// ComposerProject represents a loaded (Docker) compose project.
type ComposerProject struct {
	composeyaml map[string]any // this compose project in canonical form.
	project     *types.Project
}

// LoadComposerProject looks in the specified “dir” for a Docker composer
// project file and loads it. This takes the several official variations of
// composer project file names into account. However, contrary to Docker's
// composer, it doesn't look into parent directories for project files and it
// doesn't take overrides into account.
//
// Passing nil for envvars disables all variable interpolation.
func LoadComposerProject(ctx context.Context, workingDir string, envvars map[string]string) (*ComposerProject, error) {
	for _, projectFilename := range composerFiles {
		if _, err := os.Stat(filepath.Join(workingDir, projectFilename)); err == nil {
			return NewComposerProject(ctx, workingDir, projectFilename, envvars)
		}
	}
	return nil, fmt.Errorf("no composer project file found in directory %s", workingDir)
}

// NewComposerProject reads the specified YAML file containing a (Docker)
// composer project and returns a ComposerProject object for it.
//
// Passing nil for envvars disables all variable interpolation.
func NewComposerProject(ctx context.Context, workingDir, name string, envvars map[string]string) (*ComposerProject, error) {
	// Configure the loader and get the YAML (with some checks).
	details, err := loader.LoadConfigFiles(ctx,
		// Caveat emptor: LoadConfigFiles does not resolve relative config paths
		// against workingDir, **facepalm**.
		[]string{filepath.Join(workingDir, name)},
		workingDir)
	if err != nil {
		return nil, fmt.Errorf("cannot load composer project file(s), reason: %w", err)
	}
	details.Environment = envvars
	reponame := filepath.Base(workingDir)
	if reponame == "" || reponame == "." || reponame == string(filepath.Separator) {
		return nil, fmt.Errorf("invalid repo name-less working dir %q", workingDir)
	}
	composeyaml, err := loader.LoadModelWithContext(ctx,
		*details,
		func(opts *loader.Options) {
			opts.SetProjectName(reponame, false)
			opts.SkipInterpolation = len(envvars) == 0
		})
	if err != nil {
		return nil, fmt.Errorf("cannot load composer project model, reason: %w", err)
	}
	lopts := loader.ToOptions(details, nil)
	lopts.SetProjectName(composeyaml["name"].(string), true)
	project, err := loader.ModelToProject(
		composeyaml,
		lopts,
		*details,
	)
	if err != nil {
		return nil, fmt.Errorf("cannot load composer project, reason: %w", err)
	}
	p := &ComposerProject{
		composeyaml: composeyaml,
		project:     project,
	}

	if err := p.checkProjectName(reponame); err != nil {
		return nil, err
	}
	if err := p.checkServiceMemlimits(); err != nil {
		return nil, err
	}

	return p, nil
}

// checkServiceMemlimits checks that all services declare memory limits
// other than zero.
func (p *ComposerProject) checkServiceMemlimits() error {
	for _, service := range p.project.Services {
		if service.MemLimit == 0 {
			return fmt.Errorf("service %q has no mem_limit declaration", service.Name)
		}
	}
	return nil
}

// checkProjectName checks that the project's name matches the detected
// repo name; otherwise, the Edge runtime will react erratically claiming an app
// wasn't started when in fact it was successfully deployed. This appears to be
// a bug in the Edge runtime detection logic.
func (p *ComposerProject) checkProjectName(reponame string) error {
	if p.project.Name != reponame {
		return fmt.Errorf("project name %q does not match detected repo name %q", p.project.Name, reponame)
	}
	return nil
}

// ServiceImages maps service names in Docker composer projects to their image
// references.
type ServiceImages map[string]string

// Images returns the mapping between services defined in this composer project
// and the container images they reference. It rejects any attempts to use the
// "latest" tag without also sha256-pinning.
func (p *ComposerProject) Images() (ServiceImages, error) {
	svcimgs := ServiceImages{}

	if p.project == nil || len(p.project.Services) == 0 {
		return nil, errors.New("no services defined")
	}
	for _, service := range p.project.Services {
		if service.Image == "" {
			return nil, fmt.Errorf("service %q with empty image reference", service.Name)
		}
		slog.Info("want image",
			slog.String("service", service.Name),
			slog.String("image", service.Image))
		imgref, err := reference.Parse(service.Image)
		if err != nil {
			return nil, fmt.Errorf("service %q with invalid image reference %q, reason: %w",
				service.Name, service.Image, err)
		}
		tagged, tagOk := imgref.(reference.NamedTagged)
		_, digestOk := imgref.(reference.Digested)
		if tagOk && tagged.Tag() == "latest" && !digestOk {
			return nil, fmt.Errorf("service %q attempts to use latest tag without digest", service.Name)
		}
		svcimgs[service.Name] = service.Image
	}
	return svcimgs, nil
}

// PullImages takes a service-to-image reference mapping and pulls and saves the
// required container images. The caller is responsible to supply the correct
// "root" directory path inside which to place the images in a “image/”
// subdirectory. That is, the root path needs to reference the arbitrarily named
// “repository” folder.
func (p *ComposerProject) PullImages(
	ctx context.Context,
	serviceimgs ServiceImages,
	platform string,
	root string,
	optclient daemon.Client,
) error {
	// As multiple services might reference the same container image and we must
	// pull an image only after we first determine the unique image references.
	uniqueImageRefs := sets.New[string]()
	for _, imageRef := range serviceimgs {
		uniqueImageRefs.Add(imageRef)
	}
	slog.Debug("fetching and tar-ball'ing images...",
		slog.Int("image-count", len(uniqueImageRefs)))
	// Prepare the images subdirectory where we will place the downloaded
	// container images and then pull ... pull ... PULL!
	imagesDir := filepath.Join(root, "images")
	if err := os.MkdirAll(imagesDir, 0777); err != nil {
		return fmt.Errorf("cannot create temporary images directory, reason: %w", err)
	}

	start := time.Now()
	for _, imageRef := range slices.Sorted(maps.Keys(uniqueImageRefs)) {
		_, err := SaveImageToFile(ctx, imageRef, platform, imagesDir, optclient)
		if err != nil {
			return fmt.Errorf("cannot pull and save image %q, reason: %w", imageRef, err)
		}
	}
	duration := time.Duration(math.Ceil(time.Since(start).Seconds())) * time.Second
	slog.Debug("all images fetched and saved",
		slog.Duration("duration", duration))
	return nil
}

// Save writes the loaded composer project to the specified io.Writer, returning
// an error in case of failure.
func (p *ComposerProject) Save(w io.Writer) error {
	slog.Debug("saving composer project...")
	b, err := yaml.Marshal(p.composeyaml)
	if err != nil {
		return fmt.Errorf("cannot write composer project, reason: %w", err)
	}
	_, err = w.Write(b)
	if err != nil {
		return fmt.Errorf("cannot write composer project, reason: %w", err)
	}
	return nil
}
