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
  - $REPO/appicon.png
  - $REPO/docker-compose.yml (or .yaml)
  - $REPO/nginx (where necessary)
  - $REPO/nginx/nginx.json
*/
package tiap
