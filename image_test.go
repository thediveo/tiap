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
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	jp "github.com/PaesslerAG/jsonpath"
	"github.com/docker/docker/api/types"
	"github.com/moby/moby/client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/thediveo/success"
)

const canaryImageRef = "busybox:latest"

var _ = Describe("image pulling and saving", Ordered, func() {

	var tmpDirPath string
	var moby *client.Client

	BeforeAll(func() {
		tmpDirPath = Successful(os.MkdirTemp("", "tiap-test-*"))
		DeferCleanup(func() { os.RemoveAll(tmpDirPath) })
		moby = Successful(client.NewClientWithOpts(
			client.WithAPIVersionNegotiation()))
		DeferCleanup(func() { moby.Close() })
	})

	When("things go south", func() {

		It("reports when context is cancelled", func() {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			Expect(SaveImageToFile(ctx, moby, canaryImageRef, tmpDirPath)).Error().
				To(MatchError(ContainSubstring("context canceled")))
		})

		It("reports when image reference is invalid", func(ctx context.Context) {
			Expect(SaveImageToFile(ctx, moby, ":", tmpDirPath)).Error().
				To(MatchError(ContainSubstring("invalid reference format")))

		})

		It("reports when image reference can't be found", func(ctx context.Context) {
			Expect(SaveImageToFile(ctx, moby, "busybox:earliest", tmpDirPath)).Error().
				To(MatchError(ContainSubstring("manifest unknown")))

		})

		It("reports when image cannot be saved", func(ctx context.Context) {
			Expect(SaveImageToFile(ctx, moby, canaryImageRef, "/nada-nothing-nil")).Error().
				To(MatchError(ContainSubstring("cannot create file for image with ID")))

		})

	})

	It("pulls an image, saves it to a .tar file and names it after the SHA256 digest value", slowSpec, func(ctx context.Context) {
		moby.ImageRemove(ctx, canaryImageRef, types.ImageRemoveOptions{
			Force:         true, // ensure test coverage
			PruneChildren: true,
		})

		filename, err := SaveImageToFile(ctx, moby, canaryImageRef, tmpDirPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(filename).To(MatchRegexp(`^[0-9a-z]{64}\.tar$`))
		Expect(filepath.Join(tmpDirPath, filename)).To(BeAnExistingFile())

		jsons := Successful(
			exec.Command("docker", "inspect", canaryImageRef).Output())
		var v any
		Expect(json.Unmarshal(jsons, &v)).To(Succeed())
		imageID := Successful(jp.Get("$[0].Id", v)).(string)
		Expect(imageID).To(HavePrefix("sha256:"))
		Expect(filename).To(Equal(strings.TrimPrefix(imageID, "sha256:") + ".tar"))
	})

})
