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
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/distribution/distribution/reference"
	"github.com/moby/moby/client"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

// https://docs.docker.com/compose/compose-file/03-compose-file/ says that
// “.yaml” is preferred over “.yml”.
var composerFiles = []string{
	"docker-compose.yaml",
	"docker-compose.yml",
}

// ComposerProject represents a loaded Docker composer project.
type ComposerProject struct {
	yaml map[string]any
}

// LoadComposerProject looks in the specified “dir” for a Docker composer
// project file and loads it. This takes the several official variations of
// composer project file names into account. However, contrary to Docker's
// composer, it doesn't look into parent directories for project files and it
// doesn't take overrides into account.
func LoadComposerProject(dir string) (*ComposerProject, error) {
	for _, projectFilename := range composerFiles {
		name := filepath.Join(dir, projectFilename)
		if _, err := os.Stat(name); err == nil {
			return NewComposerProject(name)
		}
	}
	return nil, fmt.Errorf("no composer project file found in directory %s", dir)
}

// NewComposerProject reads the specified YAML file containing a (Docker)
// composer project and returns a ComposerProject object for it.
func NewComposerProject(path string) (*ComposerProject, error) {
	yamltext, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read composer project, reason: %w", err)
	}
	p := &ComposerProject{}
	if err := yaml.Unmarshal(yamltext, &p.yaml); err != nil {
		return nil, fmt.Errorf("malformed composer project, reason: %w", err)
	}
	return p, nil
}

// ServiceImages maps service names in Docker composer projects to their image
// references.
type ServiceImages map[string]string

// Images returns the mapping between services defined in this composer project
// and the container images they reference.
func (p *ComposerProject) Images() (ServiceImages, error) {
	svcimgs := ServiceImages{}

	services, err := lookupMap(p.yaml, "services")
	if err != nil {
		return nil, fmt.Errorf("no services found, reason: %w", err)
	}
	for serviceName := range services {
		config, err := lookupMap(services, serviceName)
		if err != nil {
			return nil, fmt.Errorf("invalid service %q, reason: %w", serviceName, err)
		}
		imageRef, err := lookupString(config, "image")
		if err != nil {
			return nil, fmt.Errorf("invalid image element in service %q, reason: %w", serviceName, err)
		}
		log.Info(fmt.Sprintf("   🛎  service %q wants 🖼  image %q", serviceName, imageRef))
		ir, err := reference.Parse(imageRef)
		if err != nil {
			return nil, fmt.Errorf("service %q with invalid image reference %q, reason: %w",
				serviceName, imageRef, err)
		}
		if tagged, ok := ir.(reference.Tagged); ok && tagged.Tag() == "latest" {
			return nil, fmt.Errorf("service %q attempts to use latest tag", serviceName)
		}
		svcimgs[serviceName] = imageRef
	}

	return svcimgs, nil
}

type nada struct{} // not "any"

// PullImages takes a service-to-image reference mapping and pulls and saves the
// required container images. The caller is responsible to supply the correct
// "root" directory path inside which to place the images in a “image/”
// subdirectory. That is, the root path needs to reference the arbitrarily named
// “repository” folder.
func (p *ComposerProject) PullImages(
	ctx context.Context,
	moby *client.Client,
	serviceimgs ServiceImages,
	root string,
) error {
	// As multiple services might reference the same container image and we must
	// pull an image only once we first determine the unique image references.
	uniqueImageRefs := map[string]nada{}
	for _, imageRef := range serviceimgs {
		uniqueImageRefs[imageRef] = nada{}
	}
	// Prepare the images subdirectory where we will place the downloaded
	// container images and then pull ... pull ... PULL!
	imagesDir := filepath.Join(root, "images")
	os.MkdirAll(imagesDir, 0777)
	for imageRef := range uniqueImageRefs {
		_, err := SaveImageToFile(ctx, moby, imageRef, imagesDir)
		if err != nil {
			return fmt.Errorf("cannot pull and save image %q, reason: %w", imageRef, err)
		}
	}
	return nil
}

// Save writes the loaded composer project to the specified io.Writer, returning
// an error in case of failure.
func (p *ComposerProject) Save(w io.Writer) error {
	b, err := yaml.Marshal(p.yaml)
	if err != nil {
		return fmt.Errorf("cannot write composer project, reason: %w", err)
	}
	_, err = w.Write(b)
	if err != nil {
		return fmt.Errorf("cannot write composer project, reason: %w", err)
	}
	return nil
}

func lookupMap(yaml map[string]any, key string) (map[string]any, error) {
	element := yaml[key]
	if element == nil {
		return nil, fmt.Errorf("no %s found in composer project", key)
	}
	m, ok := element.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s in composer project is not an associative array", key)
	}
	return m, nil
}

func lookupString(yaml map[string]any, key string) (string, error) {
	element := yaml[key]
	if element == nil {
		return "", fmt.Errorf("no %s found in composer project", key)
	}
	s, ok := element.(string)
	if !ok {
		return "", fmt.Errorf("%s in composer project is not a string", key)
	}
	return s, nil
}
