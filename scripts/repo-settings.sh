#!/bin/sh
# Aplica la configuración versionada del repo con `gh api`: ajustes de merge, funciones de
# seguridad, el Environment integration (solo desde main) y el ruleset de main
# (.github/rulesets/main.json), que se crea o se actualiza por nombre. Idempotente: se puede
# ejecutar las veces que haga falta. Necesita gh, jq y permisos de admin sobre el repo. Se prueba con scripts/test-repo-settings.sh.
# Uso: sh scripts/repo-settings.sh [OWNER/REPO]
set -eu

root="$(cd "$(dirname "$0")/.." && pwd)"
repo="${1:-DavidJuanes/gadis-cli}"
ruleset="$root/.github/rulesets/main.json"

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

# El ruleset va al final: exige checks que ya deben existir.
name="$(jq -r .name "$ruleset")"
id="$(gh api "repos/$repo/rulesets?includes_parents=false" --paginate \
  --jq ".[] | select(.name == \"$name\") | .id")"
if [ -n "$id" ]; then
  gh api -X PUT "repos/$repo/rulesets/$id" --input "$ruleset" --silent
  echo "Ruleset \"$name\" actualizado ($id)."
else
  gh api -X POST "repos/$repo/rulesets" --input "$ruleset" --silent
  echo "Ruleset \"$name\" creado."
fi
