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
	"context"
	"os"
	"testing"
	"time"

	"github.com/moby/moby/client"
	"github.com/sirupsen/logrus"
	"golang.org/x/time/rate"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/thediveo/success"
)

var slowSpec = NodeTimeout(120 * time.Second)

// rate limit pulling images, especially when multiple unit tests need to pull
// the same image over and over again.
var pullLimiter = rate.NewLimiter(rate.Every(2*time.Second), 1)

func GrabLog(level logrus.Level) {
	origLevel := logrus.GetLevel()
	logrus.SetOutput(GinkgoWriter)
	logrus.SetLevel(level)
	DeferCleanup(func() {
		logrus.SetLevel(origLevel)
		logrus.SetOutput(os.Stderr)
	})
}

const (
	canaryImageRef = "public.ecr.aws/docker/library/busybox:latest"
)

var canaryPlatform string

var _ = BeforeSuite(func(ctx context.Context) {
	moby := Successful(client.NewClientWithOpts(client.WithAPIVersionNegotiation()))
	defer moby.Close()
	info := Successful(moby.Info(ctx))
	arch := info.Architecture
	switch arch {
	case "x86_64":
		arch = "amd64"
	case "aarch64":
		arch = "arm64"
	}
	canaryPlatform = info.OSType + "/" + arch
})

func TestLinuxKernelNamespaces(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "tiap package")
}
