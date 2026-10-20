#!/bin/sh
# Guarda de la release: release.yml la ejecuta como primer paso y aborta la publicación si el
# tag no es vX.Y.Z estricto o si su commit no es alcanzable desde origin/main. Trabaja sobre el
# clon actual, que necesita el historial completo (fetch-depth: 0). Se prueba con
# scripts/test-release-guard.sh.
# Uso: sh scripts/release-guard.sh TAG
set -eu

tag="${1:?uso: sh scripts/release-guard.sh TAG}"

# abort MENSAJE: sale con error; el prefijo ::error:: lo destaca como anotación en Actions.
abort() {
  echo "::error::$*" >&2
  exit 1
}

# vX.Y.Z con números sin ceros a la izquierda y sin sufijos: las pre-releases no se publican.
num='(0|[1-9][0-9]*)'
printf '%s\n' "$tag" | grep -Eqx "v$num\.$num\.$num" ||
  abort "El tag $tag no es vX.Y.Z (p. ej. v1.2.3, sin ceros a la izquierda ni sufijos): no se publica."

commit="$(git rev-parse --verify --quiet "refs/tags/$tag^{commit}")" ||
  abort "El tag $tag no existe en este clon."
git merge-base --is-ancestor "$commit" refs/remotes/origin/main ||
  abort "El commit $commit del tag $tag no es alcanzable desde main: las releases solo se publican desde main."

echo "El tag $tag es vX.Y.Z y su commit está en main: se puede publicar."
