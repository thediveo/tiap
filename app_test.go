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

	"github.com/sirupsen/logrus"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/thediveo/once"
	. "github.com/thediveo/success"
)

var _ = Describe("IE app building", func() {

	Context("IE app details", func() {

		It("rejects a missing or app details", func() {
			Expect(writeDetails("testdata/details/malformed/missing.json", "", "", "", "")).NotTo(Succeed())
			Expect(writeDetails("testdata/details/malformed/detail.json", "", "", "", "")).NotTo(Succeed())
		})

		When("setting and writing details", Ordered, func() {

			const semver = "11.22.33-foobar0"

			var (
				details []byte
				tmpPath string
			)

			BeforeEach(func() {
				GrabLog(logrus.InfoLevel)
				details = Successful(os.ReadFile("testdata/details/good/detail.json"))
				tmpDetails := Successful(os.CreateTemp("", "details-*.json"))
				tmpPath = tmpDetails.Name()
				closeOnce := Once(func() {
					tmpDetails.Close()
				}).Do
				DeferCleanup(func() {
					closeOnce()
					Expect(os.Remove(tmpPath)).To(Succeed())
				})
				Expect(tmpDetails.Write(details)).Error().To(Succeed())
				closeOnce()
			})

			It("updates app details with version", func() {
				Expect(writeDetails(tmpPath, "hellorld", semver, "notes", "")).To(Succeed())
				details = Successful(os.ReadFile(tmpPath))
				var d map[string]any
				Expect(json.Unmarshal([]byte(details), &d)).To(Succeed())
				Expect(d).To(HaveKeyWithValue("versionNumber", semver))
				Expect(d).To(HaveKeyWithValue("versionId", MatchRegexp(`^[0-9a-zA-Z]{32}$`)))
				Expect(d).To(HaveKeyWithValue("releaseNotes", "notes"))
				Expect(d).NotTo(HaveKey("arch"))
			})

			It("doesn't set the default architecture", func() {
				Expect(writeDetails(tmpPath, "hellorld", semver, "notes", DefaultIEAppArch)).To(Succeed())
				details = Successful(os.ReadFile(tmpPath))
				var d map[string]any
				Expect(json.Unmarshal([]byte(details), &d)).To(Succeed())
				Expect(d).NotTo(HaveKey("arch"))
			})

			It("sets the default architecture based on (non-default) platform", func() {
				Expect(writeDetails(tmpPath, "hellorld", semver, "notes", "arm64")).To(Succeed())
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
			GrabLog(logrus.InfoLevel)
			Expect(NewApp("/nothing-nada-nil")).Error().To(MatchError(
				ContainSubstring("cannot copy app template structure")))
		})

		It("reports missing repo directory", func() {
			GrabLog(logrus.InfoLevel)
			Expect(NewApp("testdata/brokenapp")).Error().To(MatchError(
				ContainSubstring("project lacks Docker compose")))
		})

		It("reports when unable to load malformed composer project", func() {
			GrabLog(logrus.InfoLevel)
			Expect(NewApp("testdata/brokencompose")).Error().To(MatchError(
				ContainSubstring("malformed composer project")))
		})

	})

	When("packaging", func() {

		It("reports error when digests cannot be stored", func() {
			GrabLog(logrus.InfoLevel)
			a := &App{tmpDir: "/nowhere"}
			Expect(a.Package("")).To(MatchError(
				ContainSubstring("cannot create digests.json")))
		})

		It("reports error when app package cannot be created", func() {
			GrabLog(logrus.InfoLevel)
			a := &App{tmpDir: "testdata/app"}
			Expect(a.Package("/nada-nothing-nil")).To(MatchError(
				ContainSubstring("cannot create IE app package file")))
		})

	})

	It("reports cancelled pull context", func() {
		GrabLog(logrus.InfoLevel)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		a := Successful(NewApp("testdata/app"))
		defer a.Done()
		Expect(a.PullAndWriteCompose(ctx, canaryPlatform, nil)).To(MatchError(
			ContainSubstring("context canceled")))
	})

	It("loads an app template, sets details, pulls, and packages", slowSpec, func(ctx context.Context) {
		GrabLog(logrus.InfoLevel)
		a := Successful(NewApp("testdata/app"))
		defer a.Done()
		Expect(a.SetDetails("1.2.3-faselblah", "", "")).To(Succeed())
		Expect(pullLimiter.Wait(ctx)).To(Succeed())
		Expect(a.PullAndWriteCompose(ctx, canaryPlatform, nil)).To(Succeed())
		Expect(a.Package("/tmp/hellorld.app")).To(Succeed())
	})

})
