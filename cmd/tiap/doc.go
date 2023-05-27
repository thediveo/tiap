/*
tiap isn't app publisher, but packages Industrial Edge .app files anyway.

# Usage

	tiap [flags] [app-template-dir]

# Flags

	    --app-version string     app semantic version, defaults to git describe
	-h, --help                   help for tiap
	-H, --host string            Docker daemon socket to connect to
	-o, --out string             mandatory: name of app package file to write
	    --release-notes string   release notes
	-v, --version                version for tiap
*/
package main
