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
	"log/slog"
	"os"

	"github.com/thediveo/tiap/test/grab"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/thediveo/success"
)

var _ = Describe("digesting digests", Ordered, func() {

	BeforeEach(func() {
		DeferCleanup(grab.Log(GinkgoWriter, slog.LevelInfo))
	})

	It("calculates correct digests of files", func() {
		digests := Successful(FileDigests("testdata/digests"))
		Expect(digests).To(And(
			HaveKeyWithValue("deetail.json",
				"2a353516432b495427291a6d8d633cbb6711b617633204cb221c8527474ae42b"),
			HaveKeyWithValue("hellorld/appicon.png",
				"e9cccf6536b48527a473cdd88569642cb37759c2611959d838ca1eb1be2db297"),
		))
	})

	It("generates digests.json content", func() {
		w := &bytes.Buffer{}
		Expect(WriteDigests(w, "testdata/digests")).To(Succeed())
		Expect(w.String()).To(MatchJSON(`{
	"version": "1",
	"files": {
		"hellorld/appicon.png": "e9cccf6536b48527a473cdd88569642cb37759c2611959d838ca1eb1be2db297",
		"deetail.json": "2a353516432b495427291a6d8d633cbb6711b617633204cb221c8527474ae42b"
	}
}`))
	})

	When("things go south", func() {

		It("reports when files cannot be opened", func() {
			badfs := &badFS{
				FS:   os.DirFS("testdata/digests"),
				fail: fsFailOpen,
			}
			Expect(fileDigests(badfs)).Error().To(
				MatchError(ContainSubstring("cannot open")))
		})

		It("reports when directories cannot be read", func() {
			badfs := &badFS{
				FS:   os.DirFS("testdata/digests"),
				fail: fsFailOpenDir,
			}
			Expect(fileDigests(badfs)).Error().To(
				MatchError(ContainSubstring("badfs open dir error")))
		})

		It("reports when files cannot be read", func() {
			badfs := &badFS{
				FS:   os.DirFS("testdata/digests"),
				fail: fsFailRead,
			}
			Expect(fileDigests(badfs)).Error().To(
				MatchError(ContainSubstring("cannot determine SHA256")))
		})

		It("reports errors when it cannot write digest data", func() {
			badw := &badWriter{}
			Expect(WriteDigests(badw, "testdata/digests")).To(
				MatchError(ContainSubstring("cannot write digests")))
		})

		It("doesn't write digests when failing to calculate them", func() {
			badfs := &badFS{
				FS:   os.DirFS("testdata/digests"),
				fail: fsFailOpen,
			}
			w := &bytes.Buffer{}
			Expect(writeDigests(w, badfs)).Error().To(
				MatchError(ContainSubstring("cannot open")))

		})

	})

})
