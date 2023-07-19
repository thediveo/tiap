# `tiap` isn't app publisher

[![PkgGoDev](https://img.shields.io/badge/-reference-blue?logo=go&logoColor=white&labelColor=505050)](https://pkg.go.dev/github.com/thediveo/tiap)
[![GitHub](https://img.shields.io/github/license/thediveo/tiap)](https://img.shields.io/github/license/thediveo/tiap)
![build and test](https://github.com/thediveo/tiap/workflows/build%20and%20test/badge.svg?branch=master)
![Coverage](https://img.shields.io/badge/Coverage-92.8%25-brightgreen)
[![Go Report Card](https://goreportcard.com/badge/github.com/thediveo/tiap)](https://goreportcard.com/report/github.com/thediveo/tiap)

`tiap` is a small Go module and CLI tool to easily create Industrial Edge `.app`
files (packages) for continuous delivery. It does nothing more than pulling the
required container images based on your app's composer project and finally
bundling all up in an `.app` package. The `.app` file then can be imported by
users into their IEM systems.

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

  However, in view of supporting IE apps for different (CPU) architectures we
  recommend to never package image files from the local daemon, but instead to
  only pull from a (remote) registry. For this, we recommend using
  `--pull-always`.

  ```bash
  go run github.com/thediveo/tiap/cmd/tiap@latest \
    -o hellorld.app --pull-always hellorldapp/
  ```
  
- no need to deal with stateful IE app publisher workspaces.

- small footprint.

Please note that `tiap` **doesn't lint** the Docker composer project, except
for:
- rejecting `:latest` image references (yes, we're more strict than IE App
    Publisher here for reasons that still hurt),
- enforcing `mem_limit` service configuration (as this seems to be the most
  common stumbling block in a survey of one sample).

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
  -h, --help                   help for tiap
  -H, --host string            Docker daemon socket to connect to (only if non-default and using local images)
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
# while in the toplevel directory of this repository...
go run github.com/thediveo/tiap/cmd/tiap@latest \
    -o hellorld.app --pull-always testdata/app/
```

outputs

```text
INFO[0000] 🗩  tiap ... isn't app publisher              
INFO[0000]    commit a56f7926 (modified)                
INFO[0000] ⚖  Apache 2.0 License                        
INFO[0000] 🏗  creating temporary project copy in "/tmp/tiap-project-1164041835" 
INFO[0000] 🫙  app repository detected as "hellorld"     
INFO[0000] 📛  semver: "v0.9.2-1-ga56f792" -> app ID: "t0CVuAlaAjzZLuoSqkI8WrblwEwUoqn1" 
INFO[0000] 🚚  pulling images and writing composer project... 
INFO[0000]    🛎  service "hellorld" wants 🖼  image "busybox:stable" 
INFO[0000]    🖭  written 5101568 bytes of 🖼  image with ID 8135583d97fe 
INFO[0000] 🌯  wrapping up...                            
INFO[0000]    🧮  determining package files SHA256 digests... 
INFO[0000]       🧮  digest(ed) detail.json: 0e684f06b98e4d68df942f410cfee52e7e03929b9ce1fca5e38e381d300e9442 
INFO[0000]       🧮  digest(ed) hellorld/appicon.png: 77911f21764738f4c4b717f7bd0371cf752128754c7f0c30d181ee5ffd6adb27 
INFO[0000]       🧮  digest(ed) hellorld/docker-compose.yml: 150cf94801f1132e34d3410358ff93860bf77eab2080c41d33ef8091901bc803 
INFO[0000]       🧮  digest(ed) hellorld/images/8135583d97feb82398909c9c97607159e6db2c4ca2c885c0b8f590ee0f9fe90d.tar: 12f0bd2e0d70f176bbcd64dc44abbb7f44b77e3abf29a327baf8c9c00a40bd55 
INFO[0000]       🧮  digest(ed) hellorld/nginx/nginx.json: 0af30ab022bed4328a143fd74eb754c8829adf62b567f1b2d9d825084d10c554 
INFO[0000]    📦  packaging detail.json                  
INFO[0000]    📦  packaging digests.json                 
INFO[0000]    📦  packaging hellorld/appicon.png         
INFO[0000]    📦  packaging hellorld/docker-compose.yml  
INFO[0000]    📦  packaging hellorld/images/8135583d97feb82398909c9c97607159e6db2c4ca2c885c0b8f590ee0f9fe90d.tar 
INFO[0000]    📦  packaging hellorld/nginx/nginx.json    
INFO[0000] ✅  ...IE app package "hellorld.app" successfully created 
INFO[0000] 🧹  removed temporary folder "/tmp/tiap-project-1164041835" 
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

## Copyright and License

Copyright 2023 Harald Albrecht, licensed under the Apache License, Version 2.0.
