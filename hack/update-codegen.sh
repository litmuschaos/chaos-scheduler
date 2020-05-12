#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

${GOPATH}/src/k8s.io/code-generator/generate-groups.sh all \
  github.com/litmuschaos/chaos-scheduler/pkg/client github.com/litmuschaos/chaos-scheduler/pkg/apis \
  litmuschaos:v1alpha1 
