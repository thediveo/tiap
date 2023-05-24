# `tiap` isn't app publisher

![Coverage](https://img.shields.io/badge/Coverage-86.8%25-brightgreen)

`tiap` is a small Go module and CLI tool to easily create Industrial Edge `.app`
files (packages) for continuous delivery. It does nothing more than pulling the
required container images based on your app's composer project and finally
bundling all up in an `.app` package.

There's no linting, except for rejecting `:latest` image references.

- Simple to automatically download and use within your pipeline.
  ```bash
  # >>> consider pinning to a specific release version <<<
  go run github.com/thediveo/tiap/cmd/tiap@latest \
    -o hellorld.app hellorldapp/
  ```
- no need to deal with stateful IE app publisher workspaces.
- small footprint.

## Copyright and License

Copyright 2023 Harald Albrecht, licensed under the Apache License, Version 2.0.
