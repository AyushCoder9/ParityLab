#!/bin/sh
set -eu

if [ "${PARITYLAB_CONFIRM_FRESH:-0}" != "1" ]; then
  echo "Refusing to remove test volumes. Re-run with PARITYLAB_CONFIRM_FRESH=1." >&2
  exit 2
fi

repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
compose_file="$repo_dir/infra/compose.yaml"
project_name=paritylab-green

cd "$repo_dir"
docker compose -p "$project_name" -f "$compose_file" down --volumes --remove-orphans
docker compose -p "$project_name" -f "$compose_file" up -d --wait

cleanup() {
  docker compose -p "$project_name" -f "$compose_file" down --volumes --remove-orphans
}
trap cleanup EXIT INT TERM

make verify

echo "fresh-volume verification passed"
