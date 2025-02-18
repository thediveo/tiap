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
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/docker/api/types/image"
	"github.com/google/go-containerregistry/pkg/name"
	ociv1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/moby/moby/client"
	"github.com/thediveo/morbyd"
	"github.com/thediveo/morbyd/pull"
	"github.com/thediveo/morbyd/timestamper"
	"github.com/thediveo/tiap/test/grab"
	"golang.org/x/sys/unix"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/thediveo/success"
)

var _ = Describe("image pulling and saving", Ordered, func() {

	var moby *client.Client

	BeforeAll(func(ctx context.Context) {
		moby = Successful(client.NewClientWithOpts(client.WithAPIVersionNegotiation()))
		DeferCleanup(func() { moby.Close() })
	})

	var tmpBundleDirPath string

	BeforeAll(func() {
		tmpBundleDirPath = Successful(os.MkdirTemp("", "tiap-test-*"))
		DeferCleanup(func() { os.RemoveAll(tmpBundleDirPath) })
	})

	When("things go south", func() {

		It("reports cancelled context", func() {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			Expect(SaveImageToFile(ctx, localCanaryImage, canaryPlatform, tmpBundleDirPath, nil)).Error().
				To(MatchError(ContainSubstring("context canceled")))
		})

		It("reports invalid platform", func(ctx context.Context) {
			Expect(SaveImageToFile(ctx, localCanaryImage, "pl/a/t/t/f/o/r:m", tmpBundleDirPath, nil)).Error().
				To(MatchError(ContainSubstring("invalid platform")))
		})

		It("reports an invalid image reference", func(ctx context.Context) {
			Expect(SaveImageToFile(ctx, ":", canaryPlatform, tmpBundleDirPath, nil)).Error().
				To(MatchError(ContainSubstring("invalid image reference")))
		})

		It("reports unknown image reference", func(ctx context.Context) {
			colon := strings.LastIndex(canaryImage, ":")
			Expect(colon).To(BeNumerically(">=", 0))
			imageref := canaryImage[:colon] + ":strangest"
			Expect(SaveImageToFile(ctx, imageref, canaryPlatform, tmpBundleDirPath, nil)).Error().
				To(MatchError(Or(
					ContainSubstring("manifest unknown"),
					ContainSubstring("MANIFEST_UNKNOWN"))))
		})

		It("reports when image cannot be saved", func(ctx context.Context) {
			Expect(SaveImageToFile(ctx, localCanaryImage, canaryPlatform, "/nada-nothing-nil", nil)).Error().
				To(MatchError(ContainSubstring("cannot create image file")))
		})

	})

	When("checking with the daemon first for a local image", func() {

		It("reports no error and returns no image if not available locally", func(ctx context.Context) {
			_, _ = moby.ImageRemove(ctx, localCanaryImage, image.RemoveOptions{
				Force:         true, // ensure test coverage
				PruneChildren: true,
			})
			Expect(hasLocalImage(ctx, moby,
				Successful(name.ParseReference(localCanaryImage)),
				Successful(ociv1.ParsePlatform(canaryPlatform)))).To(BeNil())
		})

		It("reports cancelled context", func() {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			Expect(hasLocalImage(ctx, moby,
				Successful(name.ParseReference(localCanaryImage)),
				Successful(ociv1.ParsePlatform(canaryPlatform)))).Error().To(HaveOccurred())
		})

		It("ignores unsatisfying platform", func(ctx context.Context) {
			sess := Successful(morbyd.NewSession(ctx))
			DeferCleanup(func(ctx context.Context) { sess.Close(ctx) })
			Expect(sess.PullImage(ctx,
				localCanaryImage,
				pull.WithPlatform(canaryPlatform),
				pull.WithOutput(timestamper.New(GinkgoWriter)))).To(Succeed())
			Expect(hasLocalImage(ctx, moby,
				Successful(name.ParseReference(localCanaryImage)),
				Successful(ociv1.ParsePlatform("frumpf/rust-v")))).To(BeNil())
		})

		It("returns local image", func(ctx context.Context) {
			sess := Successful(morbyd.NewSession(ctx))
			DeferCleanup(func(ctx context.Context) { sess.Close(ctx) })
			Expect(sess.PullImage(ctx,
				localCanaryImage,
				pull.WithPlatform(canaryPlatform),
				pull.WithOutput(timestamper.New(GinkgoWriter)))).To(Succeed())
			img := Successful(hasLocalImage(ctx, moby,
				Successful(name.ParseReference(localCanaryImage)),
				Successful(ociv1.ParsePlatform(canaryPlatform))))
			Expect(img).NotTo(BeNil())
		})

	})

	It("grabs an image, saves it to a .tar file and names it after the SHA256 of the image ref", slowSpec, func(ctx context.Context) {
		defer grab.Log(GinkgoWriter, slog.LevelDebug)()

		filename, err := SaveImageToFile(ctx,
			localCanaryImage, canaryPlatform, tmpBundleDirPath, nil /* ensure pull */)
		Expect(err).NotTo(HaveOccurred())
		Expect(filename).To(MatchRegexp(`^[0-9a-z]{64}\.tar$`))
		Expect(filepath.Join(tmpBundleDirPath, filename)).To(BeAnExistingFile())

		digester := sha256.New()
		digester.Write([]byte(localCanaryImage))
		Expect(filename).To(Equal(hex.EncodeToString(digester.Sum(nil)) + ".tar"))
	})

	It("reports image writing problems", func(ctx context.Context) {
		// okay, this test is now getting slightly bizare, but only slightly...
		var currrl unix.Rlimit
		Expect(unix.Getrlimit(unix.RLIMIT_FSIZE, &currrl)).To(Succeed())
		defer func() {
			Expect(unix.Setrlimit(unix.RLIMIT_FSIZE, &currrl)).To(Succeed())
		}()
		Expect(unix.Setrlimit(unix.RLIMIT_FSIZE, &unix.Rlimit{
			Cur: 100, Max: currrl.Max})).To(Succeed())

		Expect(SaveImageToFile(ctx,
			canaryImage, canaryPlatform, tmpBundleDirPath, nil /* ensure pull */)).Error().To(HaveOccurred())
	})

	Context("nil demon client", func() {

		It("does not crash when looking for a local image given a typed nil client", func(ctx context.Context) {
			var moby *client.Client // a typed nil
			Expect(func() {
				_, _ = hasLocalImage(ctx,
					moby,
					Successful(name.ParseReference("x-foobar-x")),
					nil)
			}).NotTo(Panic())
		})

		It("does not crash when looking for a local image given a non-nil client", func(ctx context.Context) {
			moby := Successful(client.NewClientWithOpts(
				client.WithAPIVersionNegotiation()))
			Expect(func() {
				_, _ = hasLocalImage(ctx,
					moby,
					Successful(name.ParseReference("x-foobar-x")),
					nil)
			}).NotTo(Panic())
			defer moby.Close()
		})

	})

})
