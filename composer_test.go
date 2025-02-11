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
	"os"

	"github.com/sirupsen/logrus"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/thediveo/success"
)

var _ = Describe("IE app composer projects", Ordered, func() {

	It("determines service images", func() {
		GrabLog(logrus.InfoLevel)
		p := Successful(NewComposerProject("testdata/composer/hellorld/docker-compose.yml"))
		imgs := Successful(p.Images())
		Expect(imgs).To(And(
			HaveKeyWithValue("bar", "alpine:edge"),
			HaveKeyWithValue("baz", "alpine:edge"),
			HaveKeyWithValue("foo", "busybox:stable"),
		))
	})

	It("automatically loads composer files .yml and .yaml", func() {
		Expect(LoadComposerProject("testdata/composer/empty")).Error().To(
			MatchError(ContainSubstring("no composer project file")))
		Expect(LoadComposerProject("testdata/composer/yaml")).Error().NotTo(HaveOccurred())
		Expect(LoadComposerProject("testdata/composer/hellorld")).Error().NotTo(HaveOccurred())
	})

	It("rejects latest image references in projects", func() {
		GrabLog(logrus.InfoLevel)
		p := Successful(LoadComposerProject("testdata/composer/latest"))
		Expect(p.Images()).Error().To(MatchError(MatchRegexp(`service .* attempts to use latest`)))
	})

	It("loads project, pulls images, writes back", slowSpec, func(ctx context.Context) {
		GrabLog(logrus.InfoLevel)

		By("setting up an empty transient testing directory")
		tmpDirPath := Successful(os.MkdirTemp("", "tiap-test-*"))
		defer os.RemoveAll(tmpDirPath)

		GrabLog(logrus.InfoLevel)

		By("loading a composer project")
		p := Successful(NewComposerProject("testdata/composer/hellorld/docker-compose.yml"))

		By("determining and pulling referenced images")
		Expect(pullLimiter.Wait(ctx)).To(Succeed())
		imgs := Successful(p.Images())
		Expect(p.PullImages(ctx, imgs, canaryPlatform, tmpDirPath, nil)).To(Succeed())
		Expect(imgs["bar"]).To(Equal(imgs["baz"]))
	})

	When("things go south", func() {

		It("reports project marshalling failures", func() {
			w := &bytes.Buffer{}
			cp := &ComposerProject{yaml: map[string]any{"bonkers": badYAMLValue{}}}
			Expect(cp.Save(w)).To(MatchError(
				ContainSubstring("bad YAML value")))
		})

		It("reports project saving failures", func() {
			w := &badWriter{}
			cp := &ComposerProject{yaml: map[string]any{"services": "none"}}
			Expect(cp.Save(w)).To(MatchError(
				ContainSubstring("cannot write composer project")))
		})

		It("reports an error when key not found", func() {
			Expect(lookupMap(map[string]any{}, "foo")).Error().To(HaveOccurred())
		})

		It("reports an error when key has a non-map value", func() {
			Expect(lookupMap(map[string]any{"foo": 42}, "foo")).Error().To(HaveOccurred())
		})

		It("reports an error when key to string not found", func() {
			Expect(lookupString(map[string]any{}, "foo")).Error().To(HaveOccurred())
		})

		It("reports an error when key has no string value", func() {
			Expect(lookupString(map[string]any{"foo": 42}, "foo")).Error().To(HaveOccurred())
		})

		It("reports missing services in project", func() {
			p := &ComposerProject{}
			Expect(p.Images()).Error().To(HaveOccurred())
		})

		It("reports invalid services in project", func() {
			GrabLog(logrus.InfoLevel)
			p := &ComposerProject{yaml: map[string]any{
				"services": map[string]any{
					"foo": 42,
				},
			}}
			Expect(p.Images()).Error().To(HaveOccurred())

			p = &ComposerProject{yaml: map[string]any{
				"services": map[string]any{
					"foo": map[string]any{},
				},
			}}
			Expect(p.Images()).Error().To(HaveOccurred())

			p = &ComposerProject{yaml: map[string]any{
				"services": map[string]any{
					"foo": map[string]any{
						"image": ":@",
					},
				},
			}}
			Expect(p.Images()).Error().To(HaveOccurred())
		})

		It("reports missing or incorrect service memory limit", func() {
			GrabLog(logrus.InfoLevel)
			p := &ComposerProject{yaml: map[string]any{
				"services": map[string]any{
					"foo": map[string]any{
						"image": "busybox:earliest",
					},
				},
			}}
			Expect(p.Images()).Error().To(MatchError(ContainSubstring("lacks mem_limit")))

			p = &ComposerProject{yaml: map[string]any{
				"services": map[string]any{
					"foo": map[string]any{
						"image":     "busybox:earliest",
						"mem_limit": "11ft8",
					},
				},
			}}
			Expect(p.Images()).Error().To(MatchError(ContainSubstring("invalid mem_limit")))
		})

		It("reports reading problems", func() {
			Expect(NewComposerProject("/")).Error().To(HaveOccurred())
			Expect(NewComposerProject("composer_test.go")).Error().To(HaveOccurred())
		})

	})

})
