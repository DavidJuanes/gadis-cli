#!/bin/sh
# Prueba scripts/confirm-tag-push.sh, el hook PreToolUse de Claude Code que pide confirmación
# antes de publicar un tag. Cada caso le pasa por stdin el JSON de PreToolUse de una orden de
# Bash y mira si pide confirmación («ask», nombrando el tag) o no dice nada.
# Uso: sh scripts/test-confirm-tag-push.sh
set -eu

. "$(dirname "$0")/testlib.sh"

# git sin la configuración del usuario, para que el resultado no dependa de la máquina.
g() {
  env GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1 git "$@"
}

# Dos repos temporales: uno sin configurar y otro con push.followTags activado.
plain="$work/plain"
follow="$work/follow"
g init -q "$plain"
g init -q "$follow"
g -C "$follow" config push.followTags true

# run CASO DIR ORDEN: ejecuta el hook con ORDEN como tool_input.command y DIR como cwd.
run() {
  jq -n --arg cmd "$3" --arg cwd "$2" \
    '{hook_event_name: "PreToolUse", cwd: $cwd, permission_mode: "default", tool_name: "Bash",
      tool_input: {command: $cmd, description: "prueba"}, tool_use_id: "toolu_test"}' >"$work/$1.json"
  capture "$work/$1" env GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1 \
    sh "$root/scripts/confirm-tag-push.sh" <"$work/$1.json"
}

# asks [TAG]: el caso $name sale con 0 y pide confirmación; si se da TAG, el motivo lo nombra.
asks() {
  [ "$(status)" = 0 ] || fail "sale con $(status), se esperaba 0"
  decision="$(jq -r '.hookSpecificOutput.permissionDecision' "$work/$name/out" 2>/dev/null || true)"
  [ "$decision" = ask ] || {
    fail "no pide confirmación"
    return
  }
  reason="$(jq -r '.hookSpecificOutput.permissionDecisionReason' "$work/$name/out")"
  echo "$reason" | grep -q irreversible || fail "el motivo no dice que es irreversible: $reason"
  [ -z "${1:-}" ] || echo "$reason" | grep -qF "$1" || fail "el motivo no nombra $1: $reason"
}

# silent: el caso $name sale con 0 sin escribir nada.
silent() {
  [ "$(status)" = 0 ] || fail "sale con $(status), se esperaba 0"
  [ ! -s "$work/$name/out" ] || fail "escribe algo y no debería"
}

# Tabla: CASO|ESPERADO|TAG|ORDEN. ESPERADO es ask o silent; TAG, el que debe nombrar el motivo.
# Todos se ejecutan en el repo sin push.followTags.
while IFS='|' read -r name want tag cmd; do
  case "$name" in '' | '#'*) continue ;; esac
  run "$name" "$plain" "$cmd"
  if [ "$want" = ask ]; then asks "$tag"; else silent; fi
done <<'TABLE'
push_tag|ask|v0.1.0|git push origin v0.1.0
push_tag_ref|ask|v0.1.0|git push origin refs/tags/v0.1.0
push_tag_keyword|ask|v0.2.0|git push origin tag v0.2.0
push_tag_refspec|ask|v0.1.0|git push origin +v0.1.0:refs/tags/v0.1.0
push_tag_quoted|ask|v0.1.0|git push origin "refs/tags/v0.1.0"
push_two_tags|ask|v0.1.1|git push origin v0.1.0 v0.1.1
push_tags|ask||git push --tags
push_tags_remote|ask||git push origin --tags
push_follow_tags|ask||git push --follow-tags origin main
push_global_option|ask|v0.1.0|git -C /tmp push origin v0.1.0
chain_and|ask|v0.1.0|git tag -a v0.1.0 -m v0.1.0 && git push origin v0.1.0
chain_semicolon|ask|v0.1.0|echo listo;git push origin v0.1.0
chain_tags|ask||go test ./... && git push --tags
push_main|silent||git push origin main
push_upstream|silent||git push -u origin rama
push_bare|silent||git push
tag_local|silent||git tag v0.1.0
tag_annotated|silent||git tag -a v0.1.0 -m "v0.1.0"
ls_remote|silent||git ls-remote --tags origin
chain_no_tag|silent||git commit -m arreglo && git push origin main
chain_tag_after_push|silent||git push origin main && git tag v0.2.0
release_create|ask|v0.1.0|gh release create v0.1.0 --title v0.1.0 --generate-notes
release_create_opts|ask|v0.1.0|gh release create --draft -t "Versión 0.1.0" v0.1.0
release_create_bare|ask||gh release create
release_list|silent||gh release list
release_view|silent||gh release view v0.1.0
api_create_ref|ask|v0.1.0|gh api repos/DavidJuanes/gadis-cli/git/refs -f ref=refs/tags/v0.1.0 -f sha=abc123
api_create_ref_method|ask|v0.1.0|gh api -X POST repos/{owner}/{repo}/git/refs -f "ref=refs/tags/v0.1.0" -f sha=abc123
api_create_tag|ask|v0.1.0|gh api --method POST repos/{owner}/{repo}/git/tags -f tag=v0.1.0 -f message=v0.1.0 -f object=abc -f type=commit
api_patch_ref|ask|v0.1.0|gh api --method=PATCH repos/{owner}/{repo}/git/refs/tags/v0.1.0 -F force=true
api_delete_ref|ask|v0.1.0|gh api -XDELETE repos/{owner}/{repo}/git/refs/tags/v0.1.0
api_input|ask||gh api repos/{owner}/{repo}/git/refs --input ref.json
api_read_ref|silent||gh api repos/{owner}/{repo}/git/refs/tags/v0.1.0
api_read_tags|silent||gh api repos/{owner}/{repo}/git/refs/tags --jq ".[].ref"
api_read_get|silent||gh api -X GET repos/{owner}/{repo}/git/refs -f per_page=100
api_write_other|silent||gh api -X POST repos/{owner}/{repo}/issues -f title=hola
sh_c|ask||sh -c "git push --tags"
title_with_tag_word|silent||git commit -m "prepara v0.1.0" && git push origin main
chain_api|ask|v0.1.0|sha=$(git rev-parse HEAD); gh api repos/{owner}/{repo}/git/refs -f ref=refs/tags/v0.1.0 -f sha=$sha
TABLE

# push.followTags: con él activado, git push sube también los tags anotados que falten en el
# remoto, así que cualquier git push puede publicar uno. Sin él, el mismo push no dice nada.
name=follow_tags_bare
run "$name" "$follow" "git push"
asks

name=follow_tags_branch
run "$name" "$follow" "git push origin main"
asks

name=follow_tags_disabled_by_flag
run "$name" "$follow" "git push --no-follow-tags origin main"
silent

name=follow_tags_via_dash_c
run "$name" "$plain" "git -C $follow push"
asks

name=follow_tags_via_cd
run "$name" "$plain" "cd $follow && git push"
asks

name=follow_tags_via_config_flag
run "$name" "$plain" "git -c push.followTags=true push"
asks

# Sin jq no se puede leer la orden: el hook pregunta ante cualquier git push o gh, y calla con
# el resto. PATH mínimo sin jq con lo que usa el hook.
nojq="$work/nojq"
mkdir -p "$nojq"
for t in sh git cat tr sort grep sed; do
  p="$(command -v "$t")"
  ln -s "$p" "$nojq/$t"
done
for c in "nojq_push|ask|git push origin main" "nojq_gh|ask|gh release create v0.1.0" "nojq_other|silent|go test ./..."; do
  name="${c%%|*}"
  rest="${c#*|}"
  run "$name.input" "$plain" "${rest#*|}"
  capture "$work/$name" env PATH="$nojq" sh "$root/scripts/confirm-tag-push.sh" <"$work/$name.input.json"
  if [ "${rest%%|*}" = ask ]; then asks; else silent; fi
done

finish confirm-tag-push.sh
