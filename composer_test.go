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
	"bytes"
	"context"
	"log/slog"
	"os"

	"github.com/thediveo/tiap/test/grab"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/thediveo/success"
)

var composerVars = map[string]string{
	"LOCAL_REGISTRY": localRegistry,
}

var _ = Describe("IE app composer projects", Ordered, func() {

	It("determines service images", func(ctx context.Context) {
		defer grab.Log(GinkgoWriter, slog.LevelDebug)()
		p := Successful(NewComposerProject(ctx,
			"testdata/composer/hellorld",
			"docker-compose.yml",
			composerVars))
		imgs := Successful(p.Images())
		Expect(imgs).To(And(
			HaveKeyWithValue("bar", HaveSuffix("/alpine:edge")),
			HaveKeyWithValue("baz", HaveSuffix("/alpine:edge")),
			HaveKeyWithValue("foo", HaveSuffix("/busybox:stable")),
		))
	})

	It("automatically loads composer files .yml and .yaml", func(ctx context.Context) {
		Expect(LoadComposerProject(ctx, "testdata/composer/empty", nil)).
			Error().To(MatchError(ContainSubstring("no composer project file")))
		Expect(LoadComposerProject(ctx, "testdata/composer/yaml", nil)).Error().NotTo(HaveOccurred())
		Expect(LoadComposerProject(ctx, "testdata/composer/hellorld", map[string]string{
			"LOCAL_REGISTRY": "notwork.example.com/",
		})).Error().NotTo(HaveOccurred())
	})

	It("rejects latest image references in projects", func(ctx context.Context) {
		defer grab.Log(GinkgoWriter, slog.LevelDebug)()
		p := Successful(LoadComposerProject(ctx, "testdata/composer/latest", nil))
		Expect(p.Images()).Error().To(MatchError(MatchRegexp(`service .* attempts to use latest`)))
	})

	It("accepts a pinned latest image references in projects", func(ctx context.Context) {
		defer grab.Log(GinkgoWriter, slog.LevelDebug)()
		p := Successful(LoadComposerProject(ctx, "testdata/composer/latestpinned", nil))
		Expect(p.Images()).Error().NotTo(HaveOccurred())
	})

	It("loads project, pulls images, writes back", slowSpec, func(ctx context.Context) {
		defer grab.Log(GinkgoWriter, slog.LevelDebug)()

		By("setting up an empty transient testing directory")
		tmpDirPath := Successful(os.MkdirTemp("", "tiap-test-*"))
		defer func() {
			Expect(os.RemoveAll(tmpDirPath)).To(Succeed())
		}()

		By("loading a composer project")
		p := Successful(NewComposerProject(ctx,
			"testdata/composer/hellorld", "docker-compose.yml", composerVars))

		By("determining and pulling referenced images")
		imgs := Successful(p.Images())
		Expect(p.PullImages(ctx, imgs, canaryPlatform, tmpDirPath, nil)).To(Succeed())
		Expect(imgs["bar"]).To(Equal(imgs["baz"]))
	})

	When("things go south", func() {

		It("reports project marshalling failures", func() {
			w := &bytes.Buffer{}
			cp := &ComposerProject{composeyaml: map[string]any{"bonkers": badYAMLValue{}}}
			Expect(cp.Save(w)).To(MatchError(
				ContainSubstring("bad YAML value")))
		})

		It("reports project saving failures", func() {
			w := &badWriter{}
			cp := &ComposerProject{composeyaml: map[string]any{"services": "none"}}
			Expect(cp.Save(w)).To(MatchError(
				ContainSubstring("cannot write composer project")))
		})

		It("reports missing services in project", func() {
			p := &ComposerProject{}
			Expect(p.Images()).Error().To(HaveOccurred())
		})

		It("reports incorrect service memory limit", func(ctx context.Context) {
			Expect(NewComposerProject(ctx,
				"testdata/composer/badlimit", "docker-compose.yml", nil)).Error().To(HaveOccurred())
		})

		It("reports a missing service memory limit", func(ctx context.Context) {
			Expect(NewComposerProject(ctx,
				"testdata/composer/nolimit", "docker-compose.yml", nil)).Error().To(HaveOccurred())
		})

		It("reports reading problems", func(ctx context.Context) {
			Expect(NewComposerProject(ctx, "/", "docker-compose.yml", nil)).Error().To(HaveOccurred())
			Expect(NewComposerProject(ctx, ".", "composer_test.go", nil)).Error().To(HaveOccurred())
		})

	})

})
