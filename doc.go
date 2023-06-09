/*
Package tiap implements a simplistic Industrial Edge .app file packager.

“tiap isn't app publisher.” In can be used instead of the Siemens Industrial
Edge App Publisher and iectl tools that are either interactive or an
un-wget-able CLI tool. In contrast, tiap is easily “go install”-able, including
version pinning. Moreover, tiap doesn't need setting up clean workspaces, et
cetera.

All that tiap needs: a “template” folder with the usual app project folder
structure. This can be easily gotten by exporting a (new) project once and
purging out the image files and the digest.json file. The structure thus is as
follows:
  - detail.json (but leave the versionNumber and versionId fields empty)
  - $REPO/
  - $REPO/appicon.png (150⨉150 pixels)
  - $REPO/docker-compose.yml (or .yaml)
  - $REPO/nginx (where necessary)
  - $REPO/nginx/nginx.json

Here, $REPO is an almost arbitrary directory name (except for “images”) that is
considered to be the app's “repository” name.

Please note that tiap doesn't lint the Docker composer project, except for:
  - rejecting “:latest” image references (yes, we're more strict than IE App
    Publisher here for a reason),
  - enforcing “mem_limit” service configuration.
*/
package tiap
