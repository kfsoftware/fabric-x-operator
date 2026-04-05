#!/bin/bash
# set -o errexit
# set -o nounset
# set -o pipefail

GOPATH=$(go env GOPATH)
SCRIPT_ROOT=$(dirname "${BASH_SOURCE[0]}")/..
CODEGEN_PKG=${CODEGEN_PKG:-$(
	cd "${SCRIPT_ROOT}"
	ls -d -1 ${SCRIPT_ROOT}/../code-generator 2>/dev/null || echo $GOPATH/pkg/mod/k8s.io/code-generator@v0.33.3
)}
OUTDIR="${SCRIPT_ROOT}/pkg/client"

echo "Generating client code under ${OUTDIR} using ${CODEGEN_PKG} ..."

source "${CODEGEN_PKG}/kube_codegen.sh"

echo "Generating helper code... for ${SCRIPT_ROOT}/api"
kube::codegen::gen_helpers \
	--boilerplate "${SCRIPT_ROOT}/k8s/boilerplate.go.txt" \
	"${SCRIPT_ROOT}/api"

echo "Generating client code... for ${SCRIPT_ROOT}/api"
kube::codegen::gen_client \
	--with-watch \
	--with-applyconfig \
	--output-pkg github.com/kfsoftware/fabric-x-operator/pkg/client \
	--output-dir "${OUTDIR}" \
	--boilerplate "${SCRIPT_ROOT}/k8s/boilerplate.go.txt" \
	"${SCRIPT_ROOT}"
