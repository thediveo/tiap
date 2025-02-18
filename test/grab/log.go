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
	"io"
	"log/slog"
)

// Log grabs any logging output and feeds it into the specified writer.
// Preferably, this writer should be a GinkgoWriter, so log output will show up
// only in case a test fails, but otherwise we stay silent. Log returns a
// function that must be deferred in order to restore the original default slog
// Logger.
func Log(w io.Writer, level slog.Level) func() {
	defaultLogger := slog.Default()
	slog.SetDefault(slog.New(
		slog.NewJSONHandler(w, &slog.HandlerOptions{
			Level: level,
		})))

	return func() {
		slog.SetDefault(defaultLogger)
	}
}
