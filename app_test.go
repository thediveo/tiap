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
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/thediveo/tiap/test/grab"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/thediveo/success"
)

var _ = Describe("IE app building", func() {

	Context("IE app details", func() {

		It("rejects a missing or app details", func() {
			Expect(setDetails("testdata/details/malformed/missing.json", "", "", "", "", nil)).NotTo(Succeed())
			Expect(setDetails("testdata/details/malformed/detail.json", "", "", "", "", nil)).NotTo(Succeed())
		})

		When("setting and writing details", Ordered, func() {

			const semver = "11.22.33-foobar0"

			var (
				details []byte
				tmpPath string
			)

			BeforeEach(func() {
				DeferCleanup(grab.Log(GinkgoWriter, slog.LevelInfo))
				details = Successful(os.ReadFile("testdata/details/good/detail.json"))
				tmpDetails := Successful(os.CreateTemp("", "details-*.json"))
				tmpPath = tmpDetails.Name()
				closeOnce := sync.OnceFunc(func() {
					tmpDetails.Close()
				})
				DeferCleanup(func() {
					closeOnce()
					Expect(os.Remove(tmpPath)).To(Succeed())
				})
				Expect(tmpDetails.Write(details)).Error().To(Succeed())
				closeOnce()
			})

			It("updates app details with version", func() {
				Expect(setDetails(tmpPath, "hellorld", semver, "notes", "", nil)).To(Succeed())
				details = Successful(os.ReadFile(tmpPath))
				var d map[string]any
				Expect(json.Unmarshal([]byte(details), &d)).To(Succeed())
				Expect(d).To(HaveKeyWithValue("versionNumber", semver))
				Expect(d).To(HaveKeyWithValue("versionId", MatchRegexp(`^[0-9a-zA-Z]{32}$`)))
				Expect(d).To(HaveKeyWithValue("releaseNotes", "notes"))
				Expect(d).NotTo(HaveKey("arch"))
			})

			It("doesn't set the default architecture", func() {
				Expect(setDetails(tmpPath, "hellorld", semver, "notes", DefaultIEAppArch, nil)).To(Succeed())
				details = Successful(os.ReadFile(tmpPath))
				var d map[string]any
				Expect(json.Unmarshal([]byte(details), &d)).To(Succeed())
				Expect(d).NotTo(HaveKey("arch"))
			})

			It("sets the default architecture based on (non-default) platform", func() {
				Expect(setDetails(tmpPath, "hellorld", semver, "notes", "arm64", nil)).To(Succeed())
				details = Successful(os.ReadFile(tmpPath))
				var d map[string]any
				Expect(json.Unmarshal([]byte(details), &d)).To(Succeed())
				Expect(d).To(HaveKeyWithValue("arch", "arm64"))
			})

		})

	})

	When("loading an IE app template", func() {

		It("reports when unable to create a temporary directory", Serial, func() {
			tmpdir := os.Getenv("TMPDIR")
			defer func() {
				os.Setenv("TMPDIR", tmpdir)
			}()
			os.Setenv("TMPDIR", "/foobar")
			Expect(NewApp("")).Error().To(MatchError(
				ContainSubstring("cannot create temporary project directory")))
		})

		It("reports when unable to read template files", func() {
			defer grab.Log(GinkgoWriter, slog.LevelInfo)()
			Expect(NewApp("/nothing-nada-nil")).Error().To(MatchError(
				ContainSubstring("cannot copy app template structure")))
		})

		It("reports missing repo directory", func() {
			defer grab.Log(GinkgoWriter, slog.LevelInfo)()
			Expect(NewApp("testdata/brokenapp")).Error().To(MatchError(
				ContainSubstring("project lacks Docker compose")))
		})

		It("reports when unable to load malformed composer project", func() {
			defer grab.Log(GinkgoWriter, slog.LevelInfo)()
			Expect(NewApp("testdata/brokencompose")).Error().To(MatchError(
				ContainSubstring("malformed composer project")))
		})

	})

	When("packaging", func() {

		It("reports error when digests cannot be stored", func() {
			defer grab.Log(GinkgoWriter, slog.LevelInfo)()
			a := &App{tmpDir: "/nowhere"}
			Expect(a.Package("")).To(MatchError(
				ContainSubstring("cannot create digests.json")))
		})

		It("reports error when app package cannot be created", func() {
			defer grab.Log(GinkgoWriter, slog.LevelInfo)()
			a := &App{tmpDir: "testdata/app"}
			Expect(a.Package("/nada-nothing-nil")).To(MatchError(
				ContainSubstring("cannot create IE app package file")))
		})

	})

	It("reports cancelled pull context", func() {
		defer grab.Log(GinkgoWriter, slog.LevelInfo)()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		a := Successful(NewApp("testdata/app"))
		Expect(a.project.Interpolate(nil)).To(Succeed())
		defer a.Done()
		Expect(a.PullAndWriteCompose(ctx, canaryPlatform, nil)).To(MatchError(
			ContainSubstring("context canceled")))
	})

	It("loads an app template, sets details, pulls, and packages", slowSpec, func(ctx context.Context) {
		defer grab.Log(GinkgoWriter, slog.LevelInfo)()
		a := Successful(NewApp("testdata/app"))
		Expect(a.project.Interpolate(map[string]string{
			"REGISTRY": localRegistry,
		})).To(Succeed())
		defer a.Done()
		Expect(a.SetDetails("1.2.3-faselblah", "", "", nil)).To(Succeed())
		Expect(a.PullAndWriteCompose(ctx, canaryPlatform, nil)).To(Succeed())
		Expect(a.Package("/tmp/hellorld.app")).To(Succeed())
	})

	It("interpolates", func() {
		defer grab.Log(GinkgoWriter, slog.LevelInfo)()
		a := Successful(NewApp("testdata/interpolated-app"))
		defer a.Done()
		vars := map[string]string{
			"IMGREF":        "latest",
			"DESCRIPTION":   "a famous description",
			"RELEASE_NOTES": "the release notes",
		}
		Expect(a.Interpolate(vars)).To(Succeed())
		Expect(a.project.yaml).To(
			HaveKeyWithValue("services",
				HaveKeyWithValue("hellorld",
					HaveKeyWithValue("image", "busybox:latest"))))
		Expect(a.SetDetails("1.2.3", "", "", vars)).To(Succeed())
		detailjson := Successful(os.ReadFile(filepath.Join(a.tmpDir, "detail.json")))
		var details map[string]any
		Expect(json.Unmarshal(detailjson, &details)).To(Succeed())
		Expect(details).To(HaveKeyWithValue("releaseNotes",
			"the release notes"))
		Expect(details).To(HaveKeyWithValue("description",
			"a famous description"))
	})

})
