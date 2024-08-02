#!/bin/sh
set -e

package=$(grep '^Source:' debian/control | awk '{print $2}')

tags=$(git tag --sort=creatordate)

prev_tag=""
for tag in $tags; do
  git checkout $tag > /dev/null 2>&1

  new_version="$(echo $tag | tr -d 'v')-1"

  export FAKETIME=$(git show -s --format=%aI $tag | sed 's/T/ /; s/.\{6\}$//')
  
  if [ -n "$prev_tag" ]; then
    LD_PRELOAD="/usr/lib/$(uname -m)-linux-gnu/faketime/libfaketime.so.1" \
      gbp dch --ignore-branch --release --distribution=stable --new-version="$new_version" --since=$prev_tag --spawn-editor=never
  else
    mkdir -p debian
    cat <<EOF > debian/changelog
$package (0.0.0-1) UNRELEASED; urgency=medium

 -- $DEBFULLNAME <$DEBEMAIL>  Thu, 01 Jan 1970 00:00:00 +0000
EOF

    LD_PRELOAD="/usr/lib/$(uname -m)-linux-gnu/faketime/libfaketime.so.1" \
      gbp dch --ignore-branch --release --distribution=stable --new-version=$new_version --spawn-editor=never
  fi

  prev_tag=$tag
done