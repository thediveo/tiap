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
	"io"
	"os"

	"github.com/sirupsen/logrus"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	//. "github.com/thediveo/success"
)

var _ = FDescribe("digesting streamed files digests", Ordered, func() {

	BeforeEach(func() {
		GrabLog(logrus.InfoLevel)
	})

	It("calculates correct digests of files", func() {
		fd := FileDigester{}
		testdatafs := os.DirFS("testdata/digests")
		Expect(fd.DigestFile(testdatafs, "deetail.json", io.Discard)).To(Succeed())
		Expect(fd.DigestFile(testdatafs, "hellorld/appicon.png", io.Discard)).To(Succeed())
		Expect(fd).To(And(
			HaveKeyWithValue("deetail.json",
				"2a353516432b495427291a6d8d633cbb6711b617633204cb221c8527474ae42b"),
			HaveKeyWithValue("hellorld/appicon.png",
				"e9cccf6536b48527a473cdd88569642cb37759c2611959d838ca1eb1be2db297"),
		))
	})

	It("generates digests.json content", func() {
		fd := FileDigester{}
		testdatafs := os.DirFS("testdata/digests")
		Expect(fd.DigestFile(testdatafs, "deetail.json", io.Discard)).To(Succeed())
		Expect(fd.DigestFile(testdatafs, "hellorld/appicon.png", io.Discard)).To(Succeed())

		w := &bytes.Buffer{}
		Expect(fd.WriteDigestsJSON(w)).To(Succeed())
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
			fd := FileDigester{}
			badfs := &badFS{
				FS:   os.DirFS("testdata/digests"),
				fail: fsFailOpen,
			}
			Expect(fd.DigestFile(badfs, "deetail.json", io.Discard)).Error().To(
				MatchError(ContainSubstring("cannot read")))
		})

		It("reports when stream cannot be read", func() {
			fd := FileDigester{}
			badfs := &badFS{
				FS:   os.DirFS("testdata/digests"),
				fail: fsFailRead,
			}
			Expect(fd.DigestFile(badfs, "deetail.json", io.Discard)).Error().To(
				MatchError(ContainSubstring("cannot determine SHA256")))
		})

		It("reports errors when it cannot stream data", func() {
			fd := FileDigester{}
			badw := &badWriter{}
			Expect(fd.DigestFile(os.DirFS("testdata/digests"), "deetail.json", badw)).To(
				MatchError(ContainSubstring("cannot determine SHA256 for")))
		})

		It("reports errors ", func() {
			fd := FileDigester{}
			badw := &badWriter{}
			Expect(fd.WriteDigestsJSON(badw)).Error().To(
				MatchError(ContainSubstring("cannot write digests JSON")))

		})

	})
})
