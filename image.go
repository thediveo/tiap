// Copyright 2023 by Harald Albrecht
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tiap

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"time"

	// legacytarball "github.com/google/go-containerregistry/pkg/legacy/tarball"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	ociv1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/daemon"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	log "github.com/sirupsen/logrus"
)

// DefaultRegistry points to the Docker registry.
var DefaultRegistry = name.DefaultRegistry

// SaveImageToFile checks if the referenced image (“imageref”) is either
// available locally for the specific platform or otherwise attempts to pull it,
// and then immediately saves it to local storage in the specified directory
// “savedir”. The name of the image file will be the image reference's SHA256.
// SaveImageToFile either reports success or a more specific error.
//
// Please note that an attempt to find the referenced image with the local
// daemon is only made when a non-nil client has been passed in. Otherwise,
// always a pull is attempted only.
//
// [go-containerregistry]: https://github.com/google/go-containerregistry
func SaveImageToFile(ctx context.Context,
	imageref string,
	platform string,
	savedir string,
	optclient daemon.Client,
) (filename string, err error) {
	log.Debugf("🐛 pulling and saving image %s to file...", imageref)
	imgRef, err := name.ParseReference(
		imageref, name.WithDefaultRegistry(DefaultRegistry))
	if err != nil {
		return "", fmt.Errorf("invalid image reference %q: %w",
			imageref, err)
	}

	wantPlatform, err := ociv1.ParsePlatform(platform)
	if err != nil {
		return "", fmt.Errorf("invalid platform %q: %w",
			platform, err)
	}
	log.Debugf("🐛 wanted platform: %s", wantPlatform)

	image, err := hasLocalImage(ctx, optclient, imgRef, wantPlatform)
	if err != nil {
		return "", err
	}
	if image == nil {
		image, err = pullRemoteImage(ctx, imgRef, wantPlatform)
		if err != nil {
			return "", err
		}
	}

	// The image save filename is the SHA256 of the imageref(!).
	digester := sha256.New()
	_, _ = digester.Write([]byte(imageref))
	filename = hex.EncodeToString(digester.Sum(nil)) + ".tar"

	// Write (rather, transfer) the container image data into the file system
	// path we were told.
	imageSavePathName := filepath.Join(savedir, filename)
	f, err := os.Create(imageSavePathName)
	if err != nil {
		return "", fmt.Errorf("cannot create image file %q, reason: %w",
			imageSavePathName, err)
	}
	defer f.Close()
	log.Debugf("🐛 writing image %s to tar-ball...", imageref)
	start := time.Now()
	//	if err := legacytarball.Write(imgRef, image, f); err != nil {
	if err := tarball.Write(imgRef, image, f); err != nil {
		log.Debugf("❌❌❌ writing image to tar-ball failed")
		return "", fmt.Errorf("cannot write image file %q, reason: %w",
			imageSavePathName, err)
	}
	totalWritten, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return "", fmt.Errorf("cannot determine length of written image file %q, reason: %w",
			imageSavePathName, err)
	}
	duration := time.Duration(math.Ceil(time.Since(start).Seconds())) * time.Second
	log.Infof("   🖭  written %d bytes of 🖼  image with ID %s in %s",
		totalWritten, filename[:12], duration)
	return
}

// hasLocalImage returns the referenced image for the specified platform, if
// available locally and using the specified daemon client. Otherwise, it
// returns a nil image and nil error if nothing was found. hasLocalImage also
// returns a nil image together with a nil error in case no daemon client was
// passed. It returns a non-nil error in case an error happened that should not
// be ignored.
func hasLocalImage(
	ctx context.Context,
	client daemon.Client,
	iref name.Reference,
	wantPlatform *ociv1.Platform,
) (ociv1.Image, error) {
	// nota bene: IsNil() panics if it gets a zero value, such as a plain nil,
	// or if the kind of value isn't chan, func, map, (unsafe) pointer,
	// interface, or slice. Usually, we would need to guard IsNil() further than
	// just shortcutting plain nil; but in this case demon.Client is an
	// interface, so the type checking at compile time ensures that we get an
	// interface value and IsNil then won't panic.
	if client == nil || reflect.ValueOf(client).IsNil() {
		log.Debugf("🐛 no client, so not checking locally")
		return nil, nil
	}
	// Is the correct image already locally available?
	log.Debugf("🐛 checking if image %s is locally available...", iref)
	image, err := daemon.Image(iref,
		daemon.WithContext(ctx), daemon.WithClient(client))
	if err != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		log.Debugf("🐛 image %s is not locally available", iref)
		return nil, nil // stay silent; no daemon, no such image, no whatever, ...
	}
	config, err := image.ConfigFile()
	if err != nil {
		// having the image matching the ref, but not being able to determine
		// its configuration is definitely something we need to report and
		// cannot ignore.
		return nil, fmt.Errorf("cannt determine configuration of image %q, reason: %w",
			iref.String(), err)
	}
	if hasPf := config.Platform(); hasPf == nil || !hasPf.Satisfies(*wantPlatform) {
		log.Debugf("🐛 image %s is not locally available (may not satisfy requested platform)", iref)
		return nil, nil
	}
	log.Debugf("🐛 image %s is locally available", iref)
	return image, nil
}

// pullRemoteImage pull the specified image for the specified platform from a
// (remote) registry. Depending on the registry, authentication might be
// necessary. We follow the tl;dr path as laid out by
// https://github.com/google/go-containerregistry/blob/main/pkg/authn/README.md.
func pullRemoteImage(
	ctx context.Context,
	imageref name.Reference,
	wantPlatform *ociv1.Platform,
) (ociv1.Image, error) {
	image, err := remote.Image(imageref,
		remote.WithContext(ctx),
		remote.WithPlatform(*wantPlatform),
		remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		return nil, fmt.Errorf("cannot pull image %s, reason: %w",
			imageref.String(), err)
	}
	return image, nil
}
