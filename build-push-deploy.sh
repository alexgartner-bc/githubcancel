#!/bin/bash

set -eo pipefail

tag=$(date +%Y-%m-%d)
image="us-west2-docker.pkg.dev/bc-prod-fusion/oci/githubcancel:${tag}"
docker build . -t $image

docker push $image
echo $image

gcloud run deploy \
    --concurrency 200 \
    --project bc-prod-fusion --region us-west2 \
    --allow-unauthenticated \
    --image $image \
    --set-env-vars=GITHUB_TOKEN=${GITHUB_TOKEN},GITHUB_OWNER=${GITHUB_OWNER},GITHUB_REPO=${GITHUB_REPO} \
    githubcancel