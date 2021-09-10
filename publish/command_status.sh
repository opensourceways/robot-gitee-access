#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

git_commit="$(git describe --tags --always --dirty)"
build_date="$(date -u '+%Y%m%d')"
docker_tag="v${build_date}-${git_commit}"
docker_name="robot-gitee-access"

cat <<EOF
STABLE_REPO ${REPO_OVERRIDE:-swr.ap-southeast-1.myhuaweicloud.com/opensourceway/}
DOCKER_TAG ${TAG_OVERRIDE:-$docker_tag}
DOCKER_NAME ${NAME_OVERRIDE:-$docker_name}
EOF
