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
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	ociv1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/daemon"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
)

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
	slog.Debug("pulling and saving image to file...",
		slog.String("path", imageref))
	imgRef, err := name.ParseReference(imageref)
	if err != nil {
		return "", fmt.Errorf("invalid image reference %q: %w",
			imageref, err)
	}

	wantPlatform, err := ociv1.ParsePlatform(platform)
	if err != nil {
		return "", fmt.Errorf("invalid platform %q: %w",
			platform, err)
	}
	slog.Debug("wanted", slog.String("platform", wantPlatform.String()))

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
	slog.Debug("writing image to tar-ball...",
		slog.String("image", imageref))
	start := time.Now()
	//	if err := legacytarball.Write(imgRef, image, f); err != nil {
	if err := tarball.Write(imgRef, image, f); err != nil {
		slog.Error("writing image to tar-ball failed",
			slog.String("error", err.Error()))
		return "", fmt.Errorf("cannot write image file %q, reason: %w",
			imageSavePathName, err)
	}
	totalWritten, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return "", fmt.Errorf("cannot determine length of written image file %q, reason: %w",
			imageSavePathName, err)
	}
	duration := time.Duration(math.Ceil(time.Since(start).Seconds())) * time.Second
	slog.Info("written image contents",
		slog.Int64("amount", totalWritten),
		slog.String("image-id", filename[:12]),
		slog.Duration("duration", duration))
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
		slog.Debug("no Docker/Moby client, so not checking locally")
		return nil, nil
	}
	// Is the correct image already locally available?
	slog.Debug("checking if image is locally available...",
		slog.String("image", iref.String()))
	image, err := daemon.Image(iref,
		daemon.WithContext(ctx), daemon.WithClient(client))
	if err != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		slog.Debug("image locally unavailable",
			slog.String("image", iref.String()))
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
		slog.Debug("image locally unavailable (may not satisfy requested platform)",
			slog.String("image", iref.String()))
		return nil, nil
	}
	slog.Debug("image is locally available",
		slog.String("image", iref.String()))
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
	slog.Debug("pulling image", slog.String("image", imageref.String()))
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
