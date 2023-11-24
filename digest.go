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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"

	log "github.com/sirupsen/logrus"
)

// StreamDigester determines the SHA256 .app file contents digests as we go
// along streaming the individual files into an .app tar file writer. After all
// files have been streamed, the StreamDigester should be told to stream the
// final digests.json information.
type StreamDigester map[string]string

func (f StreamDigester) DigestFile(root fs.FS, path string, w io.Writer) error {
	r, err := root.Open(path)
	if err != nil {
		return fmt.Errorf("cannot read %q, reason: %w", path, err)
	}
	defer r.Close()
	return f.DigestStream(path, r, w)
}

// DigestStream copies a stream from the specied reader to the specified writer,
// determining the stream's content digest along the way and remembering the
// final digest for later use.
func (f StreamDigester) DigestStream(path string, r io.Reader, w io.Writer) error {
	digester := sha256.New()
	w = io.MultiWriter(digester, w)
	if _, err := io.Copy(w, r); err != nil {
		return fmt.Errorf("cannot determine SHA256 for %q, rason: %w", path, err)
	}
	digest := hex.EncodeToString(digester.Sum(nil))
	f[path] = digest
	log.Info(fmt.Sprintf("      🧮  digest(ed) %q: %s", path, digest))
	return nil
}

// WriteDigestsJSON write the all calculated digests in the IE app package
// format's "digests.json" format. Just for the record: the digests.json file
// isn't digested, for reasons.
func (f StreamDigester) WriteDigestsJSON(w io.Writer) error {
	b, err := json.Marshal(struct {
		Version string            `json:"version"`
		Files   map[string]string `json:"files"`
	}{
		Version: "1",
		Files:   f,
	})
	if err != nil {
		return fmt.Errorf("cannot generate digests JSON, reason: %w", err)
	}
	_, err = w.Write(b)
	if err != nil {
		return fmt.Errorf("cannot write digests JSON, reason: %w", err)
	}
	return nil
}
