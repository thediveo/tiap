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
	"fmt"
	"strings"
	"time"

	"github.com/moby/moby/client"
	"github.com/thediveo/morbyd"
	"github.com/thediveo/morbyd/pull"
	"github.com/thediveo/morbyd/push"
	"github.com/thediveo/morbyd/run"
	"github.com/thediveo/morbyd/session"
	"github.com/thediveo/morbyd/timestamper"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/thediveo/success"
)

// The TCP port where we expose our local container registry on loopback (only).
const registryPort = 5999

// The upstream original image reference to get from the official Docker
// registry.
const originalImage = "busybox:latest"

// The image and tag we'll use in our tests, and with the default registry
// reconfigured. On purpose, we use a different tag as to not interfere with the
// original upstream image tags.
const canaryImage = "busybox:weirdest"

// The local registry authn to use for image pushes; arbitrary, but must be
// non-empty base64 (or so).
const magic = "deadbeef"

var localRegistry = fmt.Sprintf("127.0.0.1:%d/", registryPort)

// full image reference to our local registry-located testing image
var localCanaryImage = localRegistry + canaryImage

var moreCanaries = []string{
	"busybox:stable",
	"alpine:edge",
}

// the platform the Docker demon is running on; we're primarily interested in
// amd64/arm64.
var canaryPlatform string

var _ = BeforeSuite(func(ctx context.Context) {
	canaryPlatform = determinePlatform(ctx)

	sess := Successful(morbyd.NewSession(ctx,
		session.WithAutoCleaning("test=tiap.registry")))
	DeferCleanup(func(ctx context.Context) {
		By("removing the local container registry")
		sess.Close(ctx)
	})

	/*
		DeferCleanup(func() {
			cmd := exec.Command("df", "-h")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			Expect(cmd.Run()).To(Succeed())
		})
	*/

	By("starting a local container registry")
	_ = Successful(sess.Run(ctx, "registry:2",
		run.WithName("local-registry"),
		run.WithPublishedPort(fmt.Sprintf("127.0.0.1:%d:5000", registryPort)),
		run.WithAutoRemove()))

	// normal PullImage will always first check instead of skipping
	// immediately, so we need to check explicitly before pulling.
	if !Successful(sess.HasImage(ctx, originalImage)) {
		Byf("pulling the canary image %q for architecture %q from upstream", originalImage, canaryPlatform)
		Expect(sess.PullImage(ctx,
			originalImage,
			pull.WithPlatform(canaryPlatform),
			pull.WithOutput(timestamper.New(GinkgoWriter)))).To(Succeed())
	} else {
		Byf("canary image %q already available", originalImage)
	}

	Byf("tagging the canary image %q for local registry", localCanaryImage)
	Expect(sess.TagImage(ctx, originalImage, localCanaryImage)).To(Succeed())

	By("pushing the canary image into the local registry")
	// We're potentially in a race condition with the local registry still
	// starting up when rerunning tests so that the canary images are already
	// pulled. So if our push request gets rejected with an EOF while gobbling
	// the service outcome, we shortly wait and then try again for a limited
	// overall time until we succeed or throw in the towel. However, we stop
	// dead at the first non-nil, non-EOF error.
	Eventually(func() error {
		err := sess.PushImage(ctx, localCanaryImage,
			push.WithRegistryAuth(magic),
			push.WithOutput(timestamper.New(GinkgoWriter)))
		if err != nil && !strings.Contains(err.Error(), "EOF") {
			return StopTrying("local registry fail: " + err.Error())
		}
		return err
	}).Within(5 * time.Second).ProbeEvery(500 * time.Millisecond).Should(Succeed())

	Byf("removing the canary image %q from the container engine", localCanaryImage)
	Expect(sess.RemoveImage(ctx, localCanaryImage)).Error().NotTo(HaveOccurred())

	By("copying more images into our local registry")
	for _, imageref := range moreCanaries {
		if !Successful(sess.HasImage(ctx, imageref)) {
			Expect(sess.PullImage(ctx,
				imageref,
				pull.WithPlatform(canaryPlatform),
				pull.WithOutput(timestamper.New(GinkgoWriter)))).To(Succeed())
		}
		Expect(sess.TagImage(ctx, imageref, localRegistry+imageref)).To(Succeed())
		Expect(sess.PushImage(ctx,
			localRegistry+imageref,
			push.WithRegistryAuth(magic),
			push.WithOutput(timestamper.New(GinkgoWriter)))).To(Succeed())
	}
})

func determinePlatform(ctx context.Context) string {
	moby := Successful(client.NewClientWithOpts(client.WithAPIVersionNegotiation()))
	defer moby.Close()

	info := Successful(moby.Info(ctx))
	arch := info.Architecture
	switch arch {
	case "x86_64":
		arch = "amd64"
	case "aarch64":
		arch = "arm64"
	default:
		Fail("unsupported architecture: " + arch)
	}
	return info.OSType + "/" + arch
}

func Byf(format string, a ...any) {
	By(fmt.Sprintf(format, a...))
}
