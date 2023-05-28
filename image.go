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
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/errdefs"
	"github.com/moby/moby/client"
	log "github.com/sirupsen/logrus"
)

// SaveImageToFile optionally pulls the referenced image (“imageref”) if not
// locally present and then saves the image to local storage in the specified
// directory “savedir”. The name of the image file will be the image's SHA256 ID
// without the “sha256:” prefix (and not to be confused with repository SHA256
// digests). SaveImageToFile either reports success or a more specific error.
func SaveImageToFile(ctx context.Context,
	moby *client.Client,
	imageref string,
	savedir string,
) (filename string, err error) {
	// Only pull if image reference isn't already available locally.
	_, _, err = moby.ImageInspectWithRaw(ctx, imageref)
	if errdefs.IsNotFound(err) {
		if err = pullImage(ctx, moby, imageref); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", fmt.Errorf("cannot check local images, reason: %w", err)
	}
	// Image is locally available now, so get its image SHA256 digest in order
	// to determine the filename that is to receive the container image data.
	imageDetails, _, err := moby.ImageInspectWithRaw(ctx, imageref)
	if err != nil {
		return "", err
	}
	imageID := imageDetails.ID
	if !strings.HasPrefix(imageID, "sha256:") {
		return "", fmt.Errorf("image ID should be an sha256 digest, but instead is %q",
			imageID)
	}
	// The image save filename is the SHA256 of the imageref.
	digester := sha256.New()
	_, _ = digester.Write([]byte(imageref))
	filename = hex.EncodeToString(digester.Sum(nil)) + ".tar"
	// Copy the container image data from Docker's image storage into the file
	// system path we were told.
	imageFilename := filepath.Join(savedir, filename)
	imageReader, err := moby.ImageSave(ctx, []string{imageref})
	if err != nil {
		return "", err
	}
	defer imageReader.Close()
	imageFile, err := os.Create(imageFilename)
	if err != nil {
		return "", fmt.Errorf("cannot create file for image with ID %s, reason: %w",
			filename[:12], err)
	}
	defer imageFile.Close()
	totalWritten, err := io.Copy(imageFile, imageReader)
	if err != nil {
		return "", fmt.Errorf("cannot save image with ID %s, reason: %w",
			filename[:12], err)
	}
	log.Info(fmt.Sprintf("   🖭  written %d bytes of 🖼  image with ID %s", totalWritten, filename[:12]))
	return
}

// pullImage checks with the referenced registry, if any, to determine whether
// it needs to pull the referenced image.
func pullImage(ctx context.Context, moby *client.Client, imageref string) error {
	// Start the image pull process (if necessary). The events will be
	// individual lines of JSON text.
	events, err := moby.ImagePull(ctx, imageref, types.ImagePullOptions{})
	if err != nil {
		return err
	}
	defer events.Close()
	// Now read JSON-encoded events from the event stream until we reach EOF:
	// this at the same time signals to us that the operation is finished (for
	// good or bad).
	scanner := bufio.NewScanner(events)
	for scanner.Scan() {
		var ev PullEvent
		if err := json.Unmarshal(scanner.Bytes(), &ev); err != nil {
			return fmt.Errorf("cannot process pull event stream, reason: %w", err)
		}
		switch ev.Status {
		case "Downloading", "Extracting":
			log.Info(fmt.Sprintf("Pull - %s %d/%d",
				ev.Status, ev.ProgressDetail.Current, ev.ProgressDetail.Total))
		default:
			if ev.ID != "" {
				log.Info(fmt.Sprintf("Pull - %s: %s", ev.Status, ev.ID))
			} else {
				log.Info(fmt.Sprintf("Pull - %s", ev.Status))
			}
		}
	}
	return nil
}

// PullEvent sent by the Docker API for pulling images, fs layers, downloading,
// et cetera, as emitted by the Docker API client's ImagePull method.
type PullEvent struct {
	ID             string         `json:"id"`
	Status         string         `json:"status"`
	ProgressDetail ProgressDetail `json:"progressDetail"`
}

// ProgressDetail used while downloading and extracting layers in order to
// report the specific progress in terms of bytes.
type ProgressDetail struct {
	Current uint `json:"current"`
	Total   uint `json:"total"`
}
