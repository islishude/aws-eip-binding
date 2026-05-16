#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TF_DIR="$ROOT_DIR/test/e2e/terraform"
ARTIFACT_DIR="$ROOT_DIR/.e2e"
ARTIFACT="$ARTIFACT_DIR/aws-eip-binding-linux-amd64"

if [[ -z "${AWS_REGION:-}" ]]; then
  echo "AWS_REGION is required, for example: AWS_REGION=us-east-1 $0" >&2
  exit 2
fi

E2E_ENABLE_IPV6="${E2E_ENABLE_IPV6:-true}"
case "$E2E_ENABLE_IPV6" in
  true|false) ;;
  *)
    echo "E2E_ENABLE_IPV6 must be either true or false" >&2
    exit 2
    ;;
esac

E2E_NAME_PREFIX="${E2E_NAME_PREFIX:-aws-eip-binding-e2e-$(date -u +%Y%m%d%H%M%S)}"
if [[ ! "$E2E_NAME_PREFIX" =~ ^[a-z0-9][a-z0-9-]{1,38}[a-z0-9]$ ]]; then
  echo "E2E_NAME_PREFIX must match ^[a-z0-9][a-z0-9-]{1,38}[a-z0-9]$" >&2
  exit 2
fi

mkdir -p "$ARTIFACT_DIR"

echo "[e2e] building Linux amd64 binary at $ARTIFACT"
(
  cd "$ROOT_DIR"
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOAMD64=v1 go build -o "$ARTIFACT" .
)

echo "[e2e] initializing Terraform in $TF_DIR"
terraform -chdir="$TF_DIR" init

echo "[e2e] running Terraform E2E test in $AWS_REGION with prefix $E2E_NAME_PREFIX"
terraform -chdir="$TF_DIR" test \
  -var="aws_region=$AWS_REGION" \
  -var="binary_path=$ARTIFACT" \
  -var="name_prefix=$E2E_NAME_PREFIX" \
  -var="enable_ipv6=$E2E_ENABLE_IPV6"
