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
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/thediveo/morbyd/timestamper"
	"github.com/thediveo/tiap/test/grab"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
		const appbundle = "/tmp/hellorld.test.app"

		var buff strings.Builder
		logw := io.MultiWriter(&buff, GinkgoWriter)
		defer grab.Log(logw, slog.LevelDebug)()

		_ = os.Remove(appbundle)

		var rootCmd *cobra.Command
		Expect(func() {
			rootCmd = New(logw)
		}).NotTo(Panic())
		rootCmd.SilenceErrors = true
		rootCmd.SetArgs([]string{
			"--out", appbundle,
			"--app-version", "v0.0.666",
			"--interpolate",
			"--debug",
			"../../../testdata/app",
		})
		os.Setenv("REGISTRY", fmt.Sprintf("127.0.0.1:%d/", registryPort)) // non-existing registry
		Expect(rootCmd.Execute()).To(Succeed())

		Expect(appbundle).To(BeARegularFile())
		appf := Successful(os.Open(appbundle))
		defer appf.Close()
		apptar := tar.NewReader(appf)
		names := []string{}
		for {
			header, err := apptar.Next()
			if err == io.EOF {
				break
			}
			Expect(err).NotTo(HaveOccurred())
			finfo := header.FileInfo()
			if finfo.IsDir() {
				continue
			}
			Expect(finfo.Mode()&^fs.ModePerm).To(Equal(fs.FileMode(0)), "not a plain file")
			Expect(finfo.Size()).NotTo(BeZero())
			names = append(names, header.Name)
		}

		Expect(names).To(ContainElements(
			"detail.json",
			"digests.json",
			"hellorld/appicon.png",
			"hellorld/nginx/nginx.json"))

		logs := buff.String()
		// as we're using tint as our slog handler, we need to keep in mind that
		// there will be ANSI escape sequences embedded into the logging
		// details...
		Expect(logs).To(MatchRegexp(`(?m)checking if image is locally available\.\.\. .*image=.*/busybox:stable
.*image locally unavailable .*image=.*/busybox:stable
.*pulling image .*image=.*/busybox:stable
`))
	})

})
