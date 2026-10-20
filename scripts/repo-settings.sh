#!/bin/sh
# Aplica la configuración versionada del repo con `gh api`: ajustes de merge, funciones de
# seguridad, el Environment integration (solo desde main), las immutable releases, las etiquetas
# breaking y skip-changelog (si faltan) y los rulesets de .github/rulesets/ (main y los de los
# tags v*), que se crean o se actualizan por nombre y target. Idempotente: se puede ejecutar las
# veces que haga falta. Necesita gh, jq y permisos de admin sobre el repo. Se prueba con
# scripts/test-repo-settings.sh.
# Uso: sh scripts/repo-settings.sh [OWNER/REPO]
set -eu

root="$(cd "$(dirname "$0")/.." && pwd)"
repo="${1:-DavidJuanes/gadis-cli}"

# Merges: solo squash, borrar la rama al mergear y auto-merge. Secret scanning con push protection
# y validity checks.
gh api -X PATCH "repos/$repo" --silent --input - <<'JSON'
{
  "allow_squash_merge": true,
  "allow_merge_commit": false,
  "allow_rebase_merge": false,
  "delete_branch_on_merge": true,
  "allow_auto_merge": true,
  "security_and_analysis": {
    "secret_scanning": { "status": "enabled" },
    "secret_scanning_push_protection": { "status": "enabled" },
    "secret_scanning_validity_checks": { "status": "enabled" }
  }
}
JSON
echo "Ajustes de merge y secret scanning aplicados."
# La API no documenta validity checks para repos personales y puede ignorarlo sin error.
validity="$(gh api "repos/$repo" --jq '.security_and_analysis.secret_scanning_validity_checks.status // "disabled"')"
if [ "$validity" != enabled ]; then
  echo "AVISO: GitHub no ha activado validity checks por la API. Actívalo a mano en" >&2
  echo "  https://github.com/$repo/settings/security_analysis (Secret Protection > Validity checks)." >&2
fi

gh api -X PUT "repos/$repo/private-vulnerability-reporting" --silent
echo "Reporte privado de vulnerabilidades activado."

# Las actualizaciones de seguridad de Dependabot necesitan las alertas activadas antes.
gh api -X PUT "repos/$repo/vulnerability-alerts" --silent
gh api -X PUT "repos/$repo/automated-security-fixes" --silent
echo "Alertas y actualizaciones de seguridad de Dependabot activadas."

# Environment integration, con las credenciales de los tests de cuenta (los secretos los añade el
# mantenedor a mano): solo se despliega desde main. Se crea o se actualiza, y de sus reglas de
# ramas solo queda main.
env="repos/$repo/environments/integration"
gh api -X PUT "$env" --silent --input - <<'JSON'
{ "deployment_branch_policy": { "protected_branches": false, "custom_branch_policies": true } }
JSON
policies="$(gh api "$env/deployment-branch-policies" --paginate \
  --jq '.branch_policies[] | "\(.id) \(.type // "branch") \(.name)"')"
echo "$policies" | while read -r pid ptype pname; do
  [ -n "$pid" ] || continue
  if [ "$ptype $pname" != "branch main" ]; then
    gh api -X DELETE "$env/deployment-branch-policies/$pid" --silent
  fi
done
if ! echo "$policies" | grep -q ' branch main$'; then
  gh api -X POST "$env/deployment-branch-policies" --silent --input - <<'JSON'
{ "name": "main", "type": "branch" }
JSON
fi
echo "Environment integration restringido a main."

# Immutable releases: una release publicada ya no cambia de tag ni de archivos. GoReleaser la
# crea como borrador, sube todo y la publica al final, así que no choca con esto.
gh api -X PUT "repos/$repo/immutable-releases" --silent
echo "Immutable releases activadas."

# Etiquetas de las notas de la release (.github/release.yml) que GitHub no trae de serie. Solo se
# crean si faltan: si ya existen, no se tocan.
labels="$(gh api "repos/$repo/labels" --paginate --jq '.[].name')"
for label in "breaking b60205 Cambio incompatible: sube el minor en 0.x" \
  "skip-changelog ededed Fuera de las notas de la release"; do
  lname="${label%% *}"
  rest="${label#* }"
  if ! echo "$labels" | grep -qx "$lname"; then
    jq -n --arg name "$lname" --arg color "${rest%% *}" --arg description "${rest#* }" \
      '{name: $name, color: $color, description: $description}' |
      gh api -X POST "repos/$repo/labels" --input - --silent
    echo "Etiqueta $lname creada."
  fi
done

# Los rulesets van al final: el de main exige checks que ya deben existir. Los tags v* llevan
# dos, porque el bypass vale para todo un ruleset: tags-v-creacion (solo el admin crea tags) y
# tags-v-inmutables (sin bypass: nadie los actualiza ni los borra).
for ruleset in "$root"/.github/rulesets/*.json; do
  name="$(jq -r .name "$ruleset")"
  target="$(jq -r '.target // "branch"' "$ruleset")"
  id="$(gh api "repos/$repo/rulesets?includes_parents=false&targets=$target" --paginate \
    --jq ".[] | select(.name == \"$name\" and .target == \"$target\") | .id")"
  if [ -n "$id" ]; then
    gh api -X PUT "repos/$repo/rulesets/$id" --input "$ruleset" --silent
    echo "Ruleset \"$name\" actualizado ($id)."
  else
    gh api -X POST "repos/$repo/rulesets" --input "$ruleset" --silent
    echo "Ruleset \"$name\" creado."
  fi
done
