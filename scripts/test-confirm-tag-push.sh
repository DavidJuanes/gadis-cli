#!/bin/sh
# Prueba scripts/confirm-tag-push.sh, el hook PreToolUse de Claude Code que pide confirmación
# antes de publicar un tag. Cada caso le pasa por stdin el JSON de PreToolUse de una orden de
# Bash y mira si pide confirmación («ask», con lo que hace la orden y el tag) o no dice nada.
# Uso: sh scripts/test-confirm-tag-push.sh
set -eu

. "$(dirname "$0")/testlib.sh"

# Dos repos temporales: uno sin configurar y otro con push.followTags activado.
plain="$work/plain"
follow="$work/follow"
isolated_git init -q "$plain"
isolated_git init -q "$follow"
isolated_git -C "$follow" config push.followTags true

# run CASO DIR ORDEN: ejecuta el hook con ORDEN como tool_input.command y DIR como cwd.
run() {
  jq -n --arg cmd "$3" --arg cwd "$2" \
    '{hook_event_name: "PreToolUse", cwd: $cwd, permission_mode: "default", tool_name: "Bash",
      tool_input: {command: $cmd, description: "prueba"}, tool_use_id: "toolu_test"}' >"$work/$1.json"
  capture "$work/$1" isolated sh "$root/scripts/confirm-tag-push.sh" <"$work/$1.json"
}

# asks VERBO [TAG]: el caso $name sale con 0 y pide confirmación con un motivo que dice VERBO
# (publica, mueve o borra) y, si se da TAG, lo nombra.
asks() {
  [ "$(status)" = 0 ] || fail "sale con $(status), se esperaba 0"
  decision="$(jq -r '.hookSpecificOutput.permissionDecision' "$work/$name/out" 2>/dev/null || true)"
  [ "$decision" = ask ] || {
    fail "no pide confirmación"
    return
  }
  reason="$(jq -r '.hookSpecificOutput.permissionDecisionReason' "$work/$name/out")"
  echo "$reason" | grep -q irreversible || fail "el motivo no dice que es irreversible: $reason"
  echo "$reason" | grep -qF "$1" || fail "el motivo no dice que $1: $reason"
  [ -z "${2:-}" ] || echo "$reason" | grep -qF "$2" || fail "el motivo no nombra $2: $reason"
}

# silent: el caso $name sale con 0 sin escribir nada.
silent() {
  [ "$(status)" = 0 ] || fail "sale con $(status), se esperaba 0"
  [ ! -s "$work/$name/out" ] || fail "escribe algo y no debería"
}

# Tabla: CASO|ESPERADO|TAG|ORDEN. ESPERADO es silent o lo que el motivo dice que hace la orden
# (publica, mueve o borra); TAG, el que debe nombrar. Todos se ejecutan en el repo sin
# push.followTags.
while IFS='|' read -r name want tag cmd; do
  case "$name" in '' | '#'*) continue ;; esac
  run "$name" "$plain" "$cmd"
  if [ "$want" = silent ]; then silent; else asks "$want" "$tag"; fi
done <<'TABLE'
push_tag|publica|v0.1.0|git push origin v0.1.0
push_tag_ref|publica|v0.1.0|git push origin refs/tags/v0.1.0
push_tag_keyword|publica|v0.2.0|git push origin tag v0.2.0
push_tag_refspec|publica|v0.1.0|git push origin +v0.1.0:refs/tags/v0.1.0
push_tag_quoted|publica|v0.1.0|git push origin "refs/tags/v0.1.0"
push_two_tags|publica|v0.1.1|git push origin v0.1.0 v0.1.1
push_tags|publica||git push --tags
push_tags_remote|publica||git push origin --tags
push_follow_tags|publica||git push --follow-tags origin main
push_mirror|publica||git push --mirror origin
push_global_option|publica|v0.1.0|git -C /tmp push origin v0.1.0
# Solo cuentan como tags v[0-9]* y refs/tags/*: ramas que empiezan por v no.
push_branch_vendor|silent||git push origin vendor-update
push_branch_version|silent||git push -u origin version-branch
push_branch_to_v|silent||git push origin HEAD:vendor
# Borrar un tag remoto: el motivo dice que lo borra, no que lo publica.
push_delete|borra|v0.1.0|git push --delete origin v0.1.0
push_delete_short|borra|v0.1.0|git push origin -d refs/tags/v0.1.0
push_delete_refspec|borra|v0.1.0|git push origin :refs/tags/v0.1.0
push_delete_branch|silent||git push origin --delete rama
# Argumentos que solo se conocen al ejecutar: ante la duda, pregunta.
push_subst|publica||git push origin $(git describe --tags --abbrev=0)
push_backticks|publica||git push origin `git describe --tags`
push_var|publica||git push origin "$TAG"
push_xargs|publica||git tag --points-at HEAD | xargs git push origin
push_xargs_opts|publica||echo v0.1.0 | xargs -n1 git push origin
chain_and|publica|v0.1.0|git tag -a v0.1.0 -m v0.1.0 && git push origin v0.1.0
chain_semicolon|publica|v0.1.0|echo listo;git push origin v0.1.0
chain_tags|publica||go test ./... && git push --tags
push_main|silent||git push origin main
push_upstream|silent||git push -u origin rama
push_bare|silent||git push
tag_local|silent||git tag v0.1.0
tag_annotated|silent||git tag -a v0.1.0 -m "v0.1.0"
ls_remote|silent||git ls-remote --tags origin
chain_no_tag|silent||git commit -m arreglo && git push origin main
chain_tag_after_push|silent||git push origin main && git tag v0.2.0
release_create|publica|v0.1.0|gh release create v0.1.0 --title v0.1.0 --generate-notes
release_create_opts|publica|v0.1.0|gh release create --draft -t "Versión 0.1.0" v0.1.0
release_create_bare|publica||gh release create
# Opciones globales de gh antes de la suborden.
release_create_repo_short|publica|v0.1.0|gh -R DavidJuanes/gadis-cli release create v0.1.0
release_create_repo_long|publica|v0.1.0|gh --repo DavidJuanes/gadis-cli release create v0.1.0
release_create_repo_eq|publica|v0.1.0|gh --repo=DavidJuanes/gadis-cli release create v0.1.0
release_list|silent||gh release list
release_list_repo|silent||gh -R DavidJuanes/gadis-cli release list
release_view|silent||gh release view v0.1.0
api_create_ref|publica|v0.1.0|gh api repos/DavidJuanes/gadis-cli/git/refs -f ref=refs/tags/v0.1.0 -f sha=abc123
api_create_ref_method|publica|v0.1.0|gh api -X POST repos/{owner}/{repo}/git/refs -f "ref=refs/tags/v0.1.0" -f sha=abc123
api_create_ref_repo|publica|v0.1.0|gh -R DavidJuanes/gadis-cli api repos/{owner}/{repo}/git/refs -f ref=refs/tags/v0.1.0 -f sha=abc
api_create_tag|publica|v0.1.0|gh api --method POST repos/{owner}/{repo}/git/tags -f tag=v0.1.0 -f message=v0.1.0 -f object=abc -f type=commit
api_patch_ref|mueve|v0.1.0|gh api --method=PATCH repos/{owner}/{repo}/git/refs/tags/v0.1.0 -F force=true
api_delete_ref|borra|v0.1.0|gh api -XDELETE repos/{owner}/{repo}/git/refs/tags/v0.1.0
api_input|publica||gh api repos/{owner}/{repo}/git/refs --input ref.json
api_read_ref|silent||gh api repos/{owner}/{repo}/git/refs/tags/v0.1.0
api_read_tags|silent||gh api repos/{owner}/{repo}/git/refs/tags --jq ".[].ref"
api_read_get|silent||gh api -X GET repos/{owner}/{repo}/git/refs -f per_page=100
api_write_other|silent||gh api -X POST repos/{owner}/{repo}/issues -f title=hola
# Crear una release por la API crea también su tag.
api_create_release|publica|v0.1.0|gh api repos/{owner}/{repo}/releases -f tag_name=v0.1.0 -f name=v0.1.0
api_create_release_method|publica|v0.1.0|gh api -X POST repos/DavidJuanes/gadis-cli/releases -F tag_name=v0.1.0
api_edit_release_tag|publica|v0.1.1|gh api -X PATCH repos/{owner}/{repo}/releases/123 -f tag_name=v0.1.1
api_list_releases|silent||gh api repos/{owner}/{repo}/releases --jq ".[].tag_name"
api_generate_notes|silent||gh api repos/{owner}/{repo}/releases/generate-notes -f tag_name=v0.1.0 -f target_commitish=abc --jq .body
# La mutación createRef de GraphQL crea refs, también tags.
api_graphql_create_ref|publica||gh api graphql -f query='mutation { createRef(input: {repositoryId: "R_1", name: "refs/tags/v0.1.0", oid: "abc"}) { ref { name } } }'
api_graphql_read|silent||gh api graphql -f query='query { repository(owner: "o", name: "r") { refs(refPrefix: "refs/tags/", first: 5) { nodes { name } } } }'
sh_c|publica||sh -c "git push --tags"
title_with_tag_word|silent||git commit -m "prepara v0.1.0" && git push origin main
chain_api|publica|v0.1.0|sha=$(git rev-parse HEAD); gh api repos/{owner}/{repo}/git/refs -f ref=refs/tags/v0.1.0 -f sha=$sha
TABLE

# push.followTags: con él activado, git push sube también los tags anotados que falten en el
# remoto, así que cualquier git push puede publicar uno. Sin él, el mismo push no dice nada.
name=follow_tags_bare
run "$name" "$follow" "git push"
asks publica

name=follow_tags_branch
run "$name" "$follow" "git push origin main"
asks publica

name=follow_tags_disabled_by_flag
run "$name" "$follow" "git push --no-follow-tags origin main"
silent

name=follow_tags_via_dash_c
run "$name" "$plain" "git -C $follow push"
asks publica

name=follow_tags_via_cd
run "$name" "$plain" "cd $follow && git push"
asks publica

name=follow_tags_via_config_flag
run "$name" "$plain" "git -c push.followTags=true push"
asks publica

# Sin jq no se puede leer la orden: el hook pregunta ante cualquier git push o gh, y calla con
# el resto. PATH mínimo sin jq con lo que usa el hook.
nojq="$work/nojq"
mkdir -p "$nojq"
for tool in sh git cat tr sort grep sed; do
  ln -s "$(command -v "$tool")" "$nojq/$tool"
done
while IFS='|' read -r name want cmd; do
  run "$name.input" "$plain" "$cmd"
  capture "$work/$name" env PATH="$nojq" sh "$root/scripts/confirm-tag-push.sh" <"$work/$name.input.json"
  if [ "$want" = silent ]; then silent; else asks "$want"; fi
done <<'TABLE'
nojq_push|publica|git push origin main
nojq_gh|publica|gh release create v0.1.0
nojq_other|silent|go test ./...
TABLE

finish confirm-tag-push.sh
