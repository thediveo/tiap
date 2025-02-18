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
	"log/slog"
	"os"
)

// FileDigests calculates the SHA256 digests of files inside the “root”
// directory and its subdirectories, and returns them as a map of filenames to
// SHA256 hex strings. The SHA256 hex strings do not contain a “sha256:”
// digist scheme prefix.
//
// Please note that symbolic links are ignored.
func FileDigests(root string) (map[string]string, error) {
	return fileDigests(os.DirFS(root))
}
func fileDigests(rootfs fs.FS) (map[string]string, error) {
	slog.Info("determining package files SHA256 digests...")
	digests := map[string]string{}

	err := fs.WalkDir(rootfs, ".", func(path string, dirEntry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if dirEntry.IsDir() || path == "digests.json" { // ...safeguard
			return nil
		}
		// Open file and calculate the SHA256 digest over its contents.
		f, err := rootfs.Open(path)
		if err != nil {
			return fmt.Errorf("cannot open %s, reason: %w", path, err)
		}
		defer f.Close()
		digester := sha256.New()
		if _, err := io.Copy(digester, f); err != nil {
			return fmt.Errorf("cannot determine SHA256 for %s, reason: %w", path, err)
		}
		digest := hex.EncodeToString(digester.Sum(nil))
		digests[path] = digest
		slog.Info("digest(ed)",
			slog.String("path", path),
			slog.String("digest", digest))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return digests, nil
}

// WriteDigests determines the file digests inside the “root” directory and its
// sub directories and then writes the results to the specified io.Writer in
// “digests.json” format.
func WriteDigests(w io.Writer, root string) error {
	return writeDigests(w, os.DirFS(root))
}

func writeDigests(w io.Writer, rootfs fs.FS) error {
	digests, err := fileDigests(rootfs)
	if err != nil {
		return err
	}
	b, err := json.Marshal(struct {
		Version string            `json:"version"`
		Files   map[string]string `json:"files"`
	}{
		Version: "1",
		Files:   digests,
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
