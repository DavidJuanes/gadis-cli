#!/bin/sh
# Prueba scripts/repo-settings.sh contra un `gh` falso que guarda el estado del repo en un
# directorio local y registra cada llamada a la API. No toca GitHub.
# Uso: sh scripts/test-repo-settings.sh
set -eu

root="$(cd "$(dirname "$0")/.." && pwd)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
fails=0
repo=DavidJuanes/gadis-cli

# gh falso: solo entiende `gh api [-X MÉTODO] RUTA [--input F] [--jq EXPR] [--paginate]`.
# Estado en $FAKE_GH_STATE: rulesets.json (lista) y ruleset-ID.json (cada ruleset).
# Cada llamada se registra en $FAKE_GH_STATE/calls como "MÉTODO RUTA".
ghbin="$work/ghbin"
mkdir -p "$ghbin"
cat >"$ghbin/gh" <<'GH'
#!/bin/sh
set -eu
s="$FAKE_GH_STATE"
[ "$1" = api ] || { echo "gh falso: solo gh api" >&2; exit 2; }
shift
method=GET; path=""; input=""; jq_expr=""; silent=""
while [ $# -gt 0 ]; do
  case "$1" in
    -X | --method) method="$2"; shift ;;
    --input) input="$2"; shift ;;
    --jq | -q) jq_expr="$2"; shift ;;
    --silent) silent=1 ;;
    --paginate) ;;
    -*) echo "gh falso: opción desconocida $1" >&2; exit 2 ;;
    *) path="$1" ;;
  esac
  shift
done
echo "$method ${path%%\?*}" >>"$s/calls"
if [ "$method ${path%%\?*}" = "${FAKE_GH_FAIL:-}" ]; then
  echo "gh: HTTP 422 (fallo simulado)" >&2
  exit 1
fi
body="$s/last-body"
if [ -n "$input" ]; then
  if [ "$input" = - ]; then cat >"$body"; else cat "$input" >"$body"; fi
fi
# GitHub no documenta validity checks en la API de repos personales: simula que lo ignora.
if [ -n "${FAKE_GH_DROP_VALIDITY:-}" ] && [ "$method" = PATCH ]; then
  jq 'del(.security_and_analysis.secret_scanning_validity_checks)' "$body" >"$s/tmp"
  mv "$s/tmp" "$body"
fi
out() {
  if [ -n "$silent" ]; then cat >/dev/null
  elif [ -n "$jq_expr" ]; then jq -r "$jq_expr"
  else cat; fi
}
[ -f "$s/rulesets.json" ] || echo '[]' >"$s/rulesets.json"
case "$method ${path%%\?*}" in
  "GET repos/$FAKE_GH_REPO/rulesets") out <"$s/rulesets.json" ;;
  "POST repos/$FAKE_GH_REPO/rulesets")
    id=$(( $(jq length "$s/rulesets.json") + 100 ))
    jq --argjson id "$id" '. + {id: $id}' "$body" >"$s/ruleset-$id.json"
    jq --argjson id "$id" --arg name "$(jq -r .name "$body")" '. + [{id: $id, name: $name}]' \
      "$s/rulesets.json" >"$s/tmp" && mv "$s/tmp" "$s/rulesets.json"
    cp "$body" "$s/body-post-ruleset"
    out <"$s/ruleset-$id.json" ;;
  "PUT repos/$FAKE_GH_REPO/rulesets/"*)
    id="${path##*/}"
    [ -f "$s/ruleset-$id.json" ] || { echo "gh falso: ruleset $id no existe" >&2; exit 1; }
    jq --argjson id "$id" '. + {id: $id}' "$body" >"$s/ruleset-$id.json"
    cp "$body" "$s/body-put-ruleset"
    out <"$s/ruleset-$id.json" ;;
  "PATCH repos/$FAKE_GH_REPO")
    [ -f "$s/repo.json" ] || echo '{}' >"$s/repo.json"
    jq -s '.[0] * .[1]' "$s/repo.json" "$body" >"$s/tmp" && mv "$s/tmp" "$s/repo.json"
    cp "$body" "$s/body-patch-repo"
    out <"$s/repo.json" ;;
  "GET repos/$FAKE_GH_REPO") out <"$s/repo.json" ;;
  "PUT repos/$FAKE_GH_REPO/private-vulnerability-reporting" | \
    "PUT repos/$FAKE_GH_REPO/vulnerability-alerts" | \
    "PUT repos/$FAKE_GH_REPO/automated-security-fixes")
    touch "$s/enabled-${path##*/}" ;;
  *) echo "gh falso: llamada inesperada $method $path" >&2; exit 1 ;;
esac
GH
chmod +x "$ghbin/gh"

# run CASO: ejecuta el script sobre el estado de $work/CASO y deja salida y código de salida.
run() {
  dir="$work/$1"
  mkdir -p "$dir/state"
  set +e
  PATH="$ghbin:$PATH" FAKE_GH_STATE="$dir/state" FAKE_GH_REPO="$repo" \
    sh "$root/scripts/repo-settings.sh" "$repo" >"$dir/out" 2>&1
  echo $? >"$dir/status"
  set -e
}

fail() {
  echo "FAIL $name: $*"
  sed 's/^/    /' "$work/$name/out"
  fails=$((fails + 1))
}
state() { echo "$work/$name/state"; }
status() { cat "$work/$name/status"; }
called() { grep -qx "$1" "$(state)/calls" 2>/dev/null; }
count() { grep -cx "$1" "$(state)/calls" 2>/dev/null || true; }

name=creates_ruleset
run "$name"
[ "$(status)" = 0 ] || fail "sale con $(status), se esperaba 0"
called "POST repos/$repo/rulesets" || fail "no crea el ruleset"
cmp -s "$(state)/body-post-ruleset" "$root/.github/rulesets/main.json" ||
  fail "no envía .github/rulesets/main.json al crear el ruleset"

# Ya hay un ruleset "main" (id 7) y otro ajeno: se actualiza el suyo, no se crea otro.
name=updates_existing_ruleset
mkdir -p "$(state)"
echo '[{"id": 3, "name": "otro"}, {"id": 7, "name": "main"}]' >"$(state)/rulesets.json"
echo '{"id": 7, "name": "main", "rules": []}' >"$(state)/ruleset-7.json"
run "$name"
[ "$(status)" = 0 ] || fail "sale con $(status), se esperaba 0"
! called "POST repos/$repo/rulesets" || fail "crea un ruleset nuevo en vez de actualizar el existente"
called "PUT repos/$repo/rulesets/7" || fail "no actualiza el ruleset 7"
cmp -s "$(state)/body-put-ruleset" "$root/.github/rulesets/main.json" ||
  fail "no envía .github/rulesets/main.json al actualizar el ruleset"

name=applies_repo_settings
run "$name"
[ "$(status)" = 0 ] || fail "sale con $(status), se esperaba 0"
expected='{"allow_auto_merge":true,"allow_merge_commit":false,"allow_rebase_merge":false,"allow_squash_merge":true,"delete_branch_on_merge":true,"push_protection":"enabled","secret_scanning":"enabled","validity_checks":"enabled"}'
got="$(jq -cS '{allow_squash_merge, allow_merge_commit, allow_rebase_merge, delete_branch_on_merge,
  allow_auto_merge, secret_scanning: .security_and_analysis.secret_scanning.status,
  push_protection: .security_and_analysis.secret_scanning_push_protection.status,
  validity_checks: .security_and_analysis.secret_scanning_validity_checks.status}' \
  "$(state)/body-patch-repo" 2>/dev/null || true)"
[ "$got" = "$expected" ] || fail "ajustes del repo: $got"
for f in private-vulnerability-reporting vulnerability-alerts automated-security-fixes; do
  [ -f "$(state)/enabled-$f" ] || fail "no activa $f"
done
# Las actualizaciones de seguridad de Dependabot exigen las alertas activadas antes.
alerts="$(grep -nx "PUT repos/$repo/vulnerability-alerts" "$(state)/calls" | cut -d: -f1 || true)"
fixes="$(grep -nx "PUT repos/$repo/automated-security-fixes" "$(state)/calls" | cut -d: -f1 || true)"
[ -n "$alerts" ] && [ -n "$fixes" ] && [ "$alerts" -lt "$fixes" ] ||
  fail "activa las actualizaciones de seguridad antes que las alertas de Dependabot"

# Dos ejecuciones seguidas: la segunda actualiza lo que creó la primera y el estado no cambia.
name=idempotent
run "$name"
[ "$(status)" = 0 ] || fail "la primera ejecución sale con $(status)"
mkdir -p "$work/snap"
cp "$(state)"/rulesets.json "$(state)"/ruleset-*.json "$(state)"/repo.json "$work/snap/"
run "$name"
[ "$(status)" = 0 ] || fail "la segunda ejecución sale con $(status)"
[ "$(count "POST repos/$repo/rulesets")" = 1 ] || fail "crea el ruleset más de una vez"
id="$(jq -r '.[0].id' "$(state)/rulesets.json")"
[ "$(count "PUT repos/$repo/rulesets/$id")" = 1 ] || fail "la segunda ejecución no actualiza el ruleset $id"
[ "$(jq length "$(state)/rulesets.json")" = 1 ] || fail "hay más de un ruleset"
for f in "$work"/snap/*; do
  cmp -s "$f" "$(state)/${f##*/}" || fail "el estado cambia en la segunda ejecución: ${f##*/}"
done

name=stops_on_api_error
FAKE_GH_FAIL="PUT repos/$repo/private-vulnerability-reporting"
export FAKE_GH_FAIL
run "$name"
unset FAKE_GH_FAIL
[ "$(status)" != 0 ] || fail "sale con 0 aunque falla una llamada a la API"
! called "PUT repos/$repo/vulnerability-alerts" || fail "sigue tras un error de la API"

name=warns_without_validity_checks
FAKE_GH_DROP_VALIDITY=1
export FAKE_GH_DROP_VALIDITY
run "$name"
unset FAKE_GH_DROP_VALIDITY
[ "$(status)" = 0 ] || fail "sale con $(status), se esperaba 0"
grep -q "validity checks" "$work/$name/out" || fail "no avisa de que validity checks sigue desactivado"
! grep -q "validity checks" "$work/applies_repo_settings/out" ||
  fail "avisa de validity checks aunque está activado"

# Cada check obligatorio del ruleset es un job de ci.yml o codeql.yml sin `name:` propio
# (el check se llama como el id del job). Si se renombra un job, esto falla.
name=required_checks_are_jobs
mkdir -p "$work/$name"
: >"$work/$name/out"
contexts="$(jq -r '.rules[] | select(.type == "required_status_checks")
  | .parameters.required_status_checks[].context' "$root/.github/rulesets/main.json")"
for want in test lint vuln secrets build analyze; do
  echo "$contexts" | grep -qx "$want" || fail "el ruleset no exige el check $want"
done
for ctx in $contexts; do
  file=""
  for wf in ci codeql; do
    awk -v job="  $ctx:" '/^jobs:/ { in_jobs = 1; next } in_jobs && $0 == job { found = 1 } END { exit !found }' \
      "$root/.github/workflows/$wf.yml" && file="$root/.github/workflows/$wf.yml"
  done
  if [ -z "$file" ]; then
    fail "el check $ctx no es ningún job de ci.yml ni codeql.yml"
  elif awk -v job="  $ctx:" '$0 == job { in_job = 1; next } in_job && /^  [^ ]/ { exit 1 }
      in_job && /^    name:/ { found = 1 } END { exit !found }' "$file"; then
    fail "el job $ctx tiene name: propio, así que su check no se llama $ctx"
  fi
done

if [ "$fails" -gt 0 ]; then
  echo "$fails fallo(s)"
  exit 1
fi
echo "repo-settings.sh: todas las pruebas pasan"
