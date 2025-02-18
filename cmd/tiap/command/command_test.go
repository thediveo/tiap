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

package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/thediveo/gtar"
	"github.com/thediveo/morbyd/timestamper"
	"github.com/thediveo/tiap/test/grab"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	. "github.com/thediveo/success"
)

var _ = Describe("tiap command", func() {

	Context("helpers", func() {

		It("panics on structural problem", func() {
			Expect(func() {
				_ = successfully(func() (bool, error) { return false, errors.New("D'OH!") }())
			}).To(Panic())
		})

		It("logs errors and exits", func() {
			oldOsExit := osExit
			defer func() { osExit = oldOsExit }()
			osExit = func(code int) { panic("D'OH!") }

			var buff strings.Builder
			defer grab.Log(&buff, slog.LevelInfo)()
			Expect(func() {
				_ = unerringly(func() (bool, error) { return false, errors.New("D'OH!!!") }())
			}).To(PanicWith("D'OH!"))
			Expect(buff.String()).To(MatchRegexp(`"msg":"fatal","error":"D'OH!!!"`))
		})

	})

	It("chokes on mis-defined env var", func() {
		logw := timestamper.New(GinkgoWriter)
		defer grab.Log(logw, slog.LevelDebug)()

		var rootCmd *cobra.Command
		Expect(func() {
			rootCmd = New(logw)
		}).NotTo(Panic())
		rootCmd.SilenceErrors = true
		rootCmd.SilenceUsage = true
		rootCmd.SetArgs([]string{
			"--out", "/tmp/hellorld.kaputt.app",
			"--app-version", "v0.0.666",
			"--interpolate",
			"--debug",
			"../../../testdata/app",
		})
		os.Setenv("REGISTRY", "127.0.0.1:1/") // non-existing registry
		Expect(rootCmd.Execute()).To(
			MatchError(ContainSubstring("connection refused")))
	})

	It("packages hellorld", slowSpec, func(ctx context.Context) {
		const appbundlePath = "/tmp/hellorld.test.app"

		var buff strings.Builder
		logw := io.MultiWriter(&buff, GinkgoWriter)
		defer grab.Log(logw, slog.LevelDebug)()

		_ = os.Remove(appbundlePath)

		var rootCmd *cobra.Command
		Expect(func() {
			rootCmd = New(logw)
		}).NotTo(Panic())
		rootCmd.SilenceErrors = true
		rootCmd.SetArgs([]string{
			"--out", appbundlePath,
			"--app-version", "v0.0.666",
			"--interpolate",
			"--debug",
			"../../../testdata/app",
		})
		os.Setenv("REGISTRY", fmt.Sprintf("127.0.0.1:%d/", registryPort)) // non-existing registry
		Expect(rootCmd.Execute()).To(Succeed())

		logs := buff.String()
		// as we're using tint as our slog handler, we need to keep in mind that
		// there will be ANSI escape sequences embedded into the logging
		// details...
		Expect(logs).To(MatchRegexp(`(?m)checking if image is locally available\.\.\. .*image=.*/busybox:stable
.*image locally unavailable .*image=.*/busybox:stable
.*pulling image .*image=.*/busybox:stable
`))

		Expect(appbundlePath).To(BeARegularFile())
		apptarIndex := Successful(gtar.New(appbundlePath))
		defer apptarIndex.Close()

		Expect(apptarIndex.AllRegularFilePaths()).To(ContainElements(
			"detail.json",
			"digests.json",
			"hellorld/appicon.png",
			"hellorld/docker-compose.yml",
			"hellorld/nginx/nginx.json"))

		var imagePaths []string
		Expect(apptarIndex.AllRegularFilePaths()).To(ContainElement(MatchRegexp(`^hellorld/images/.*\.tar$`), &imagePaths))
		Expect(imagePaths).To(HaveLen(1))

		digestsf := Successful(apptarIndex.Open("digests.json"))
		defer digestsf.Close()

		var digests struct {
			Files map[string]string `json:"files"`
		}
		Expect(json.NewDecoder(digestsf).Decode(&digests)).To(Succeed())
		Expect(digests.Files).NotTo(BeEmpty())

		Expect(imagePaths).To(HaveEach(BeKeyOf(digests.Files)))

		imageDigestMap := FilterKeys(digests.Files, MatchRegexp(`^hellorld/images/.*\.tar$`))
		Expect(imageDigestMap).To(HaveLen(1))
		Expect(imageDigestMap).To(HaveEach(Not(BeZero())))
	})

})

func FilterKeys[M ~map[K]V, K comparable, V any](m M, matcher types.GomegaMatcher) M {
	result := M{}
	for k, v := range m {
		match, err := matcher.Match(k)
		if err != nil {
			Fail(err.Error())
		}
		if !match {
			continue
		}
		result[k] = v
	}
	return result
}
