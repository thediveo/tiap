# `tiap` isn't app publisher

[![PkgGoDev](https://img.shields.io/badge/-reference-blue?logo=go&logoColor=white&labelColor=505050)](https://pkg.go.dev/github.com/thediveo/tiap)
[![GitHub](https://img.shields.io/github/license/thediveo/tiap)](https://img.shields.io/github/license/thediveo/tiap)
![build and test](https://github.com/thediveo/tiap/actions/workflows/buildandtest.yaml/badge.svg?branch=master)
![Coverage](https://img.shields.io/badge/Coverage-91.9%25-brightgreen)
[![Go Report Card](https://goreportcard.com/badge/github.com/thediveo/tiap)](https://goreportcard.com/report/github.com/thediveo/tiap)

`tiap` is a small Go module and CLI tool to easily create Industrial Edge `.app`
files (packages) for continuous delivery. It does nothing more than pulling the
required container images based on your app's composer project and finally
bundling all up in an `.app` package. The `.app` file then can be imported by
users into their IEM systems.

## Features

- simple to automatically download and use within your pipeline:
  ```bash
  # >>> consider pinning tiap to a specific release version <<<
  go run github.com/thediveo/tiap/cmd/tiap@latest \
    -o hellorld.app hellorldapp/
  ```

- defaults to using `git describe` to set the app version, or set explicitly
  using `--app-version $SEMVER`. Even accepts `v` prefixed semvers and then
  drops the prefix.

- talks to the Docker API _socket_, so there's no need to either reconfigure the
  Docker daemon in your dev system or in pipelines, or to fiddle around with
  `socat` to reroute a localhost TCP port to the Docker socker.

  However, in view of supporting IE apps for different (CPU) architectures **we
  recommend to never package image files from the local daemon**, but instead to
  only pull from a (remote) registry. For this, we recommend sticking to
  `--pull-always`.

  ```bash
  go run github.com/thediveo/tiap/cmd/tiap@latest \
    -o hellorld.app --pull-always hellorldapp/
  ```
  
- environment variable interpolation in both the composer file, as well as the
  `details.json` (see details below). 

- no need to deal with stateful IE app publisher workspaces.

- small footprint.

## Compose Project Safety Checks

Please note that while `tiap` **doesn't lint** the Docker composer project, it
still does the following safety checks:

- rejecting `:latest` image references (yes, we're more strict than IE App
    Publisher here for reasons that still hurt),

- enforcing `mem_limit` service configuration (as this seems to be the most
  common stumbling block in a survey of one sample).

## Environment Variable Interpolation

`tiap` supports variable interpolation for both the composer file, as well as
`details.json`. It supports:

- `$FOO`
- `${FOO}`
  - `${FOO:-default}` and `${FOO-default}`
  - `${FOO:?err}` and `${FOO?err}`
  - `${FOO:+replacement}` and `${FOO+replacement}` (this is useful in such cases
    as adding optional newline padding when a certain addition text env var has
    been defined, et cetera.)

`tiap` interpolates using the environment variables it was started with; there
is no support for `.env` files so far (PR welcome).

Please note in case of `details.json`:
- `versionNumber` is always set via the `--version` CLI flag. If the value to
  `--version` is empty (`""`), `tiap` runs [`git
  describe`](https://git-scm.com/docs/git-describe) on the current repository to
  find the most recent tag that is reachable from HEAD.
- `versionID` is automatically determined from he SHA256 hash of `--version` and
  the repository name.
- `releaseNotes` can be set using `--release-notes` and in this case no further
  interpolation occurs on the release notes; instead, the caller needs to
  interpolate any environment variables before executing `tiap`. But if
  `--release-notes` isn't used or an empty value (`""`) specified, then
  interpolation occurs immediately after reading `details.json` and the result
  is kept.

## Note

> [!IMPORTANT]
> As of v0.11.0, tiap creates the image tar-balls inside .app files in
> _non_-legacy format, in order to be able to properly process newer (base)
> images. This requires a moderately recent IEM and is known to work with
> IE(v)D versions 1.19 and later.

## CLI

The command

```bash
tiap -h
```

outputs

```text
tiap isn't app publisher, but packages Industrial Edge .app files anyway

Usage:
  tiap -o FILE [flags] APP-TEMPLATE-DIR

Flags:
      --app-version string     app semantic version, defaults to git describe
      --debug                  enable debug logging
  -h, --help                   help for tiap
  -H, --host string            Docker daemon socket to connect to (only if non-default and using local images)
  -i, --interpolate            interpolate env vars in compose project and detail.json
  -o, --out string             mandatory: name of app package file to write
  -p, --platform string        platform to build app for (default "linux/amd64")
      --pull-always            always pull image from remote registry, never use local images
      --release-notes string   release notes (interpreted as double-quoted Go string literal; use \n, \", …)
  -v, --version                version for tiap
```

## Hellorld Demo

This packages a `hellorld.app`: when deployed, it runs an HTTP server in a
busybox container, serving the (in)famous ["Hellorld!"
greeting](https://www.youtube.com/watch?v=_j2L6nkO8MQ&t=1053s), but in text
only.

```bash
# while in the toplevel directory of this repository:
go run github.com/thediveo/tiap/cmd/tiap@latest \
    -o hellorld.app --pull-always --interpolate testdata/app/
# note: --interpolate interpolates env vars in the compose project file.
```

outputs

```text
2025-13-42T26:92:88Z INF tiap ... isn't app publisher version=(devel) license="Apache 2.0"
2025-13-42T26:92:88Z INF creating temporary project copy path=/tmp/tiap-project-2262943179
2025-13-42T26:92:88Z INF app repository detected repo=hellorld
2025-13-42T26:92:88Z INF normalized platform platform=linux/amd64
2025-13-42T26:92:88Z INF denormalized IE App architecture arch=x86-64
2025-13-42T26:92:88Z INF updated version ID based on semver semver=0.13.1-5-g7ab7cd8 versionId=Bor6mbv1fchBFRPTgFedPoFpjzuXgmK0
2025-13-42T26:92:88Z INF pulling images...
2025-13-42T26:92:88Z INF want image service=hellorld image=busybox:stable
2025-02-18T12:17:15Z INF written image contents amount=2156544 image-id=c107da89b447 duration=1s
2025-02-18T12:17:15Z INF images successfully pulled
2025-02-18T12:17:15Z INF writing final compose project...
2025-02-18T12:17:15Z INF final compose project written
2025-02-18T12:17:15Z INF wrapping up...
2025-02-18T12:17:15Z INF determining package files SHA256 digests...
2025-02-18T12:17:15Z INF digest(ed) path=detail.json digest=00d31aa9510fbf481b5d556c3b2ee0b08821f5948fbe48203ae703b800f4ac55
2025-02-18T12:17:15Z INF digest(ed) path=hellorld/appicon.png digest=77911f21764738f4c4b717f7bd0371cf752128754c7f0c30d181ee5ffd6adb27
2025-02-18T12:17:15Z INF digest(ed) path=hellorld/docker-compose.yml digest=4dde434ebf1a6450e74a63f4035974c4ce725abd9830142205913222a8725635
2025-02-18T12:17:15Z INF digest(ed) path=hellorld/images/c107da89b4470c4ed8fcaa56395fd11a0b85c57e1b6217e749655dfde1a9a91b.tar digest=558fc1edbf893ffb9abf1119dc0e4d236bdb6f165ff3d55f886aad66f82e0ea3
2025-02-18T12:17:15Z INF digest(ed) path=hellorld/nginx/nginx.json digest=d817b3a87c15f5c4807deb6e6ebf9e8aa2be5735c944f8b97a00124035c67cd5
2025-02-18T12:17:15Z INF creating IE app tar-ball doctor=Tarr professor=Fether
2025-02-18T12:17:15Z INF packaging path=detail.json
2025-02-18T12:17:15Z INF packaging path=digests.json
2025-02-18T12:17:15Z INF packaging path=hellorld
2025-02-18T12:17:15Z INF packaging path=hellorld/appicon.png
2025-02-18T12:17:15Z INF packaging path=hellorld/docker-compose.yml
2025-02-18T12:17:15Z INF packaging path=hellorld/images
2025-02-18T12:17:15Z INF packaging path=hellorld/images/c107da89b4470c4ed8fcaa56395fd11a0b85c57e1b6217e749655dfde1a9a91b.tar
2025-02-18T12:17:15Z INF packaging path=hellorld/nginx
2025-02-18T12:17:15Z INF packaging path=hellorld/nginx/nginx.json
2025-02-18T12:17:15Z INF IE app package successfully created
2025-02-18T12:17:15Z INF IE app package path=/tmp/hellorld.app duration=1s
2025-02-18T12:17:15Z INF removed temporary folder path=/tmp/tiap-project-2262943179
```

## App Template

The recommended way to set up your app "template" structure to be used by `tiap`
for packaging is to simply design your app project in IE App Publisher once and
then immediately export it. Then unpack the `.app` file (it's a plain `tar`
after all). Delete the `images` directory and `digests.json`. The rest should be
checked into your git repository.

See also `testdata/app` for our canonical "Hellorld!" example.

The sweet size for app icons seem to be 150×150 pixels and they must be in PNG
format.

## App Architecture/Platform

In order to package an .app file for an architecture other than `amd64` (_cough_
`x86-64` _cough_) use the `--platform` (or `-p`) flag. Its value can be a proper
OCI platform specification, such as `linux/amd64`, or just an architecture
specification like `arm64`. Aliases like `x86-64` are understood and
automatically normalized.

When packaging IE app files for multiple architectures we recommend – following
Docker and OCI best practises – to only build multi-arch images and push them
into a (sometimes private) registry. `tiap` will automatically pull the correct
layers based on the platform setting.

Please note that `tiap` will default to the architecture `tiap` itself _runs_
on, unless explicitly told otherwise using `--platform`!

Also, please note that the Industrial Edge platform requires the same app for
multiple architectures to be fully separate apps: the "appId" in `detail.json`
as well as the "app repository" (please see also the [Creating a new Edge
App](https://docs.eu1.edge.siemens.cloud/develop_an_application/ieap/creating_a_new_edge_app.html)
documentation) thus _must_ differ (the latter, for instance, `hellorld` and
`hellorld-arm64`). 

## App Release Notes

The `--release-note` option interprets its value as a [double-quoted Go
string](https://go.dev/ref/spec#String_literals). To add newlines, use the `\n`
escape or alternatively pass a literal newline (consult your shell on how to
pass literal newlines). Double quotes must be always escaped as `\"`.

However, be careful that your shell isn't messing around with your escaping on
its own.

## Tinkering

When tinkering with the `tiap` source code base, the recommended way is now a
devcontainer environment. The devcontainer specified in this repository
contains:
- `Docker-in-Docker`
- `gocover` command to run all tests with coverage, updating the README coverage
  badge automatically after successful runs.
- Go package documentation is served in the background on port TCP/HTTP `6060`
  of the devcontainer.
- [`go-mod-upgrade`](https://github.com/oligot/go-mod-upgrade)
- [`goreportcard-cli`](https://github.com/gojp/goreportcard).
- [`pin-github-action`](https://github.com/mheap/pin-github-action) for
  maintaining Github Actions.

## Copyright and License

Copyright 2023 Harald Albrecht, licensed under the Apache License, Version 2.0.
