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

package platform

import (
	"context"
	"errors"

	"github.com/moby/moby/client"
)

func Detect(ctx context.Context) (string, error) {
	moby, err := client.New()
	if err != nil {
		return "", err
	}
	defer func() { _ = moby.Close() }()

	info, err := moby.Info(ctx, client.InfoOptions{})
	if err != nil {
		return "", err
	}
	arch := info.Info.Architecture
	switch arch {
	case "x86_64":
		arch = "amd64"
	case "aarch64":
		arch = "arm64"
	default:
		return "", errors.New("unsupported architecture: " + arch)
	}
	return info.Info.OSType + "/" + arch, nil
}
