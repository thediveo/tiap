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
	"errors"
	"io/fs"
)

// badYAMLValue causes the YAML marshaller to throw up.
type badYAMLValue nada

func (b badYAMLValue) MarshalYAML() (interface{}, error) { return nil, errors.New("bad YAML value") }

// badWriter only throws errors on any write attempt.
type badWriter struct{}

func (w *badWriter) Write(p []byte) (n int, err error) { return 0, errors.New("snafu") }

// fsFailureMode controls which badFS operation will fail.
type fsFailureMode int

const (
	fsFailOpenDir = iota
	fsFailOpen
	fsFailRead
)

// badFS fails on eihter opening a directory, opening a file, or alternatively
// on reading a file (not directory).
type badFS struct {
	fs.FS
	fail fsFailureMode
}

func (f *badFS) Open(name string) (fs.File, error) {
	fsf, err := f.FS.Open(name)
	if err != nil {
		return nil, err
	}
	stat, err := fsf.Stat()
	if err != nil {
		fsf.Close()
		return nil, err
	}
	if stat.IsDir() {
		if f.fail == fsFailOpenDir {
			fsf.Close()
			return nil, errors.New("badfs open dir error")
		}
		return fsf, nil
	}
	if f.fail == fsFailOpen {
		fsf.Close()
		return nil, errors.New("badfs open error")
	}
	return &badFile{fsf}, nil
}

// badFile throws error on reading.
type badFile struct {
	fs.File
}

func (f *badFile) Read([]byte) (int, error) {
	return 0, errors.New("badfile read error")
}
