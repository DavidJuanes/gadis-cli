#!/bin/sh
# Prueba scripts/release-guard.sh sobre un clon temporal: un repo «remoto» con main y una rama
# aparte, clonado como lo deja actions/checkout con fetch-depth: 0. No toca la red.
# Uso: sh scripts/test-release-guard.sh
set -eu

. "$(dirname "$0")/testlib.sh"

# Remoto: dos commits en main y uno en la rama otra, que sale de main.
remote="$work/remote"
isolated_git init -q -b main "$remote"
isolated_git -C "$remote" commit -q --allow-empty -m uno
isolated_git -C "$remote" commit -q --allow-empty -m dos
isolated_git -C "$remote" checkout -q -b otra
isolated_git -C "$remote" commit -q --allow-empty -m fuera
isolated_git -C "$remote" checkout -q main

# Clon con el historial completo y los tags que se prueban, en main o en la rama otra.
clone="$work/clone"
isolated_git clone -q "$remote" "$clone"
isolated_git -C "$clone" tag -a v0.1.0 -m v0.1.0 origin/main
isolated_git -C "$clone" tag -a v10.20.30 -m v10.20.30 origin/main~1
isolated_git -C "$clone" tag -a v0.2.0 -m v0.2.0 origin/otra
for tag in v1.0 v01.2.3 v0.2.0-rc.1 1.2.3; do
  isolated_git -C "$clone" tag -a "$tag" -m "$tag" origin/main
done

# run CASO TAG: ejecuta la guarda en el clon y deja salida y estado en $work/CASO.
run() {
  (cd "$clone" && capture "$work/$1" isolated sh "$root/scripts/release-guard.sh" "$2")
}

output_has() { grep -q "$1" "$work/$name/out"; }

name=valid_tag_on_main
run "$name" v0.1.0
[ "$(status)" = 0 ] || fail "sale con $(status), se esperaba 0"

name=valid_tag_on_older_main_commit
run "$name" v10.20.30
[ "$(status)" = 0 ] || fail "sale con $(status), se esperaba 0"

name=valid_tag_off_main
run "$name" v0.2.0
[ "$(status)" != 0 ] || fail "sale con 0, se esperaba un error"
output_has "no es alcanzable desde main" || fail "no dice que el commit no está en main"

# Tags sobre main pero sin el formato vX.Y.Z estricto (sin ceros a la izquierda ni sufijos).
for tag in v1.0 v01.2.3 v0.2.0-rc.1 1.2.3; do
  name="bad_format_$tag"
  run "$name" "$tag"
  [ "$(status)" != 0 ] || fail "sale con 0, se esperaba un error"
  output_has "no es vX.Y.Z" || fail "no dice que el tag no tiene el formato vX.Y.Z"
done

finish release-guard.sh
