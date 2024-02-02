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
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/google/go-containerregistry/pkg/name"
	ociv1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/moby/moby/client"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/thediveo/once"
	. "github.com/thediveo/success"
)

var _ = Describe("image pulling and streaming", Ordered, func() {

	var moby *client.Client

	BeforeAll(func(ctx context.Context) {
		moby = Successful(client.NewClientWithOpts(client.WithAPIVersionNegotiation()))
		DeferCleanup(func() { moby.Close() })
	})

	BeforeAll(func() {
		canaryImgRef := Successful(name.ParseReference(canaryImageRef))
		if reg := canaryImgRef.Context().RegistryStr(); reg != "" {
			oldDefaultRegistry := DefaultRegistry
			DeferCleanup(func() { DefaultRegistry = oldDefaultRegistry })
			DefaultRegistry = reg
		}
	})

	When("things go south", func() {

		It("reports cancelled context", func() {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			Expect(SaveImageToTarWriter(ctx, nil, canaryImageRef, canaryPlatform, "", nil)).Error().
				To(MatchError(ContainSubstring("context canceled")))
		})

		It("reports invalid platform", func(ctx context.Context) {
			Expect(SaveImageToTarWriter(ctx, nil, canaryImageRef, "pl/a/t/t/f/o/r:m", "", nil)).Error().
				To(MatchError(ContainSubstring("invalid platform")))
		})

		It("reports an invalid image reference", func(ctx context.Context) {
			Expect(SaveImageToTarWriter(ctx, nil, ":", canaryPlatform, "", nil)).Error().
				To(MatchError(ContainSubstring("invalid image reference")))
		})

		It("reports unknown image reference", func(ctx context.Context) {
			imageref := strings.TrimSuffix(canaryImageRef, ":latest") + ":earliest"
			Expect(SaveImageToTarWriter(ctx, nil, imageref, canaryPlatform, "", nil)).Error().
				To(MatchError(Or(
					ContainSubstring("manifest unknown"),
					ContainSubstring("MANIFEST_UNKNOWN"))))
		})

		It("reports when image cannot be saved", func(ctx context.Context) {
			Expect(pullLimiter.Wait(ctx)).To(Succeed())
			var fw failingWriter
			Expect(SaveImageToTarWriter(ctx, tar.NewWriter(&fw), canaryImageRef, canaryPlatform, "/foobar", nil)).Error().
				To(MatchError(ContainSubstring("cannot add intermediate temporary image v1 tarball")))
		})

	})

	When("checking with the daemon first for a local image", func() {

		It("reports no error and returns no image if not available locally", func(ctx context.Context) {
			_, _ = moby.ImageRemove(ctx, canaryImageRef, types.ImageRemoveOptions{
				Force:         true, // ensure test coverage
				PruneChildren: true,
			})
			Expect(hasLocalImage(ctx, moby,
				Successful(name.ParseReference(canaryImageRef)),
				Successful(ociv1.ParsePlatform(canaryPlatform)))).To(BeNil())
		})

		It("reports cancelled context", func() {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			Expect(hasLocalImage(ctx, moby,
				Successful(name.ParseReference(canaryImageRef)),
				Successful(ociv1.ParsePlatform(canaryPlatform)))).Error().To(HaveOccurred())
		})

		It("ignores unsatisfying platform", func(ctx context.Context) {
			Expect(pullLimiter.Wait(ctx)).To(Succeed())
			r := Successful(moby.ImagePull(ctx, canaryImageRef, types.ImagePullOptions{
				Platform: canaryPlatform,
			}))
			closeOnce := Once(func() {
				r.Close()
			}).Do
			defer closeOnce()
			buff := &bytes.Buffer{}
			Expect(io.Copy(buff, r)).Error().NotTo(HaveOccurred())
			closeOnce()
			Expect(hasLocalImage(ctx, moby,
				Successful(name.ParseReference(canaryImageRef)),
				Successful(ociv1.ParsePlatform("frumpf/rust-v")))).To(BeNil())
		})

		It("returns local image", func(ctx context.Context) {
			Expect(pullLimiter.Wait(ctx)).To(Succeed())
			r := Successful(moby.ImagePull(ctx, canaryImageRef, types.ImagePullOptions{
				Platform: canaryPlatform,
			}))
			closeOnce := Once(func() {
				r.Close()
			}).Do
			defer closeOnce()
			buff := &bytes.Buffer{}
			Expect(io.Copy(buff, r)).Error().NotTo(HaveOccurred())
			closeOnce()
			img := Successful(hasLocalImage(ctx, moby,
				Successful(name.ParseReference(canaryImageRef)),
				Successful(ociv1.ParsePlatform(canaryPlatform))))
			Expect(img).NotTo(BeNil())
		})

	})

	It("grabs an image, writes it into a .tarball and names it after the SHA256 of the image ref", slowSpec, func(ctx context.Context) {
		GrabLog(logrus.DebugLevel)

		Expect(pullLimiter.Wait(ctx)).To(Succeed())

		fTmp := Successful(os.CreateTemp("", "test-tarball-*"))
		defer func() {
			Expect(fTmp.Close()).To(Succeed())
			Expect(os.Remove(fTmp.Name())).To(Succeed())
		}()

		Expect(SaveImageToTarWriter(ctx,
			tar.NewWriter(fTmp),
			canaryImageRef, canaryPlatform, "/foobar", nil /* ensure pull */)).
			To(Succeed())
		Expect(fTmp.Seek(0, io.SeekStart)).Error().NotTo(HaveOccurred())
		h := Successful(tar.NewReader(fTmp).Next())
		digester := sha256.New()
		digester.Write([]byte(canaryImageRef))
		Expect(h.Name).To(Equal("/foobar/images/" + hex.EncodeToString(digester.Sum(nil)) + ".tar"))
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

		fTmp := Successful(os.CreateTemp("", "test-tarball-*"))
		defer func() {
			Expect(fTmp.Close()).To(Succeed())
			Expect(os.Remove(fTmp.Name())).To(Succeed())
		}()

		Expect(pullLimiter.Wait(ctx)).To(Succeed())

		Expect(SaveImageToTarWriter(ctx,
			tar.NewWriter(fTmp),
			canaryImageRef, canaryPlatform, "/foobar", nil /* ensure pull */)).Error().To(HaveOccurred())
	})

})

type failingWriter struct{}

func (f *failingWriter) Write(p []byte) (int, error) { return 0, errors.New("sorry, we're closed") }
