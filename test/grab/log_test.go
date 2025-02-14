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

package grab

import (
	"log/slog"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("grabing default logging output", func() {

	It("restores the original logger", func() {
		defaultLogger := slog.Default()
		Log(GinkgoWriter, slog.LevelDebug)() // immediately restore
		Expect(slog.Default()).To(BeIdenticalTo(defaultLogger))
	})

	It("temporarily sets a new logger", func() {
		defaultLogger := slog.Default()
		defer Log(GinkgoWriter, slog.LevelDebug)() // immediately restore
		Expect(slog.Default()).NotTo(BeIdenticalTo(defaultLogger))
	})

	It("writes to the correct writer", func() {
		var buff strings.Builder
		defer Log(&buff, slog.LevelInfo)()
		slog.Debug("ouch")
		Expect(buff.String()).To(BeEmpty())
		slog.Info("foobar")
		Expect(buff.String()).To(ContainSubstring(`"level":"INFO","msg":"foobar"`))
	})

})
