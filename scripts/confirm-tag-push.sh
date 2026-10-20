#!/bin/sh
# Hook PreToolUse de Claude Code (registrado en .claude/settings.json con matcher Bash): pide
# confirmación antes de cualquier orden que publique un tag, porque subir un tag v* dispara
# release.yml y la release es irreversible. Lee de stdin el JSON de PreToolUse y mira
# tool_input.command. Si la orden publica un tag, escribe un JSON con permissionDecision «ask» y
# un motivo que nombra el tag; si no, no escribe nada. Siempre sale con 0. Ante la duda, pregunta.
# Se prueba con scripts/test-confirm-tag-push.sh.
set -eu

# ask MOTIVO: pide confirmación con MOTIVO y sale.
ask() {
  printf '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"ask","permissionDecisionReason":"%s"}}\n' "$1"
  exit 0
}

input="$(cat)"

# Sin jq no se puede leer la orden: se pregunta ante cualquier cosa que parezca un git push o
# una orden de gh, que es mejor que dejar pasar un tag.
if ! command -v jq >/dev/null 2>&1; then
  if printf '%s' "$input" | grep -Eq 'git[^"]* push|(^|[^a-z0-9_-])gh '; then
    ask "No hay jq para leer la orden y puede publicar un tag: subir un tag v* dispara release.yml y la release es irreversible. Confírmalo solo si no publica nada o si quieres publicarla."
  fi
  exit 0
fi

# Parte la orden en órdenes simples (por &&, ||, ;, |, &, subshells y saltos de línea), una por
# línea con las palabras separadas por US (\037) y sin comillas. Las cadenas entre comillas se
# vuelven a partir como órdenes, por si son un sh -c "git push ...". Basta para reconocer las
# palabras; no es un intérprete de sh y no ejecuta nada.
us="$(printf '\037')"
segments="$(printf '%s' "$input" | jq -r '
  def words: [scan("(?:[^\\s;&|()`\"'"'"']|\"(?:[^\"\\\\]|\\\\.)*\"|'"'"'[^'"'"']*'"'"')+|[;&|()`\\n]")]
    | reduce .[] as $w ([[]];
        if ($w | test("^[;&|()`\\n]$")) then . + [[]]
        else .[-1] += [$w | gsub("[\"'"'"'\\\\]"; "")] end)
    | .[] | select(length > 0) | join("\u001f");
  (.tool_input.command // "") as $c
  | ($c | words),
    ($c | scan("\"(?:[^\"\\\\]|\\\\.)*\"|'"'"'[^'"'"']*'"'"'") | .[1:-1] | words)
')"
# Directorio en el que corre la orden; un cd dentro de la cadena lo cambia.
dir="$(printf '%s' "$input" | jq -r '.cwd // ""')"
[ -n "$dir" ] || dir="$PWD"

# Tags que publica la orden, separados por espacios; publishes=1 si publica alguno.
tags=""
publishes=0

# found [TAG]: apunta que la orden publica un tag y, si se conoce, cuál.
found() {
  publishes=1
  [ -z "${1:-}" ] || tags="$tags ${1#refs/tags/}"
}

# git_push ARG...: analiza los argumentos de un git push que corre en $gitdir con las opciones
# -c de $gitconfig.
git_push() {
  follow=""
  while [ $# -gt 0 ]; do
    case "$1" in
      --tags | --follow-tags | --mirror) found ;;
      --no-follow-tags) follow=false ;;
      # Opciones que llevan valor en la palabra siguiente: el valor no es un refspec.
      -o | --push-option | --repo | --receive-pack | --exec) [ $# -lt 2 ] || shift ;;
      -*) ;;
      *)
        # Refspec [+]ORIGEN[:DESTINO]: cuenta como tag si alguno de los lados lo es.
        for side in "${1%%:*}" "${1#*:}"; do
          side="${side#+}"
          case "$side" in v* | refs/tags/*) found "$side" ;; esac
        done
        ;;
    esac
    shift
  done
  # Con push.followTags, cualquier push sube los tags anotados que falten en el remoto.
  if [ -z "$follow" ]; then
    # shellcheck disable=SC2086 # $gitconfig son palabras -c CLAVE=VALOR
    follow="$(git -C "$gitdir" $gitconfig config --type=bool --get push.followTags 2>/dev/null || true)"
  fi
  [ "$follow" != true ] || found
}

# gh_release_create ARG...: el tag es el primer argumento que no es una opción.
gh_release_create() {
  while [ $# -gt 0 ]; do
    case "$1" in
      # Opciones con valor en la palabra siguiente.
      -t | --title | -n | --notes | -F | --notes-file | --target | --discussion-category | \
        --notes-start-tag | -R | --repo) [ $# -lt 2 ] || shift ;;
      -*) ;;
      *)
        found "$1"
        return
        ;;
    esac
    shift
  done
  found
}

# gh_api ARG...: cuenta si escribe (método distinto de GET, o campos sin método) sobre git/refs
# o git/tags. El tag sale de la ruta (git/refs/tags/TAG) o de los campos ref= y tag=.
gh_api() {
  method=""
  fields=0
  path=""
  named=""
  while [ $# -gt 0 ]; do
    case "$1" in
      -X | --method) [ $# -lt 2 ] || { method="$2"; shift; } ;;
      -X* | --method=*) method="${1#-X}" method="${method#--method=}" ;;
      -f | -F | --field | --raw-field)
        fields=1
        [ $# -lt 2 ] || {
          case "$2" in ref=refs/tags/* | tag=*) named="$named ${2#*=}" ;; esac
          shift
        }
        ;;
      --input) fields=1 && { [ $# -lt 2 ] || shift; } ;;
      -H | --header | -q | --jq | -t | --template | --hostname | -p | --preview | --cache)
        [ $# -lt 2 ] || shift
        ;;
      -*) ;;
      *) [ -n "$path" ] || path="$1" ;;
    esac
    shift
  done
  [ -n "$method" ] || { [ "$fields" = 0 ] && method=GET || method=POST; }
  method="$(printf '%s' "$method" | tr '[:lower:]' '[:upper:]')"
  [ "$method" != GET ] && [ "$method" != HEAD ] || return 0
  case "$path" in
    */git/refs/tags/* | git/refs/tags/*) found "${path##*/git/refs/tags/}" ;;
    *git/refs* | *git/tags*) ;;
    *) return 0 ;;
  esac
  for t in $named; do found "$t"; done
  found
}

# segment PALABRA...: analiza una orden simple de la cadena.
segment() {
  case "${1:-}" in
    cd)
      case "${2:-}" in
        /*) dir="$2" ;;
        '') dir="$HOME" ;;
        *) dir="$dir/$2" ;;
      esac
      return
      ;;
  esac
  while [ $# -gt 0 ]; do
    case "$1" in
      git)
        shift
        gitdir="$dir"
        gitconfig=""
        # Opciones globales de git antes de la suborden (git -C DIR push ...).
        while [ $# -gt 0 ]; do
          case "$1" in
            -C) [ $# -lt 2 ] || { case "$2" in /*) gitdir="$2" ;; *) gitdir="$gitdir/$2" ;; esac; shift; } ;;
            -c) [ $# -lt 2 ] || { gitconfig="$gitconfig -c $2"; shift; } ;;
            --git-dir | --work-tree | --namespace) [ $# -lt 2 ] || shift ;;
            -*) ;;
            *) break ;;
          esac
          shift
        done
        if [ "${1:-}" = push ]; then
          shift
          git_push "$@"
          return
        fi
        ;;
      gh)
        if [ "${2:-}" = release ] && [ "${3:-}" = create ]; then
          shift 3
          gh_release_create "$@"
          return
        elif [ "${2:-}" = api ]; then
          shift 2
          gh_api "$@"
          return
        fi
        ;;
    esac
    [ $# -eq 0 ] || shift
  done
}

set -f
while IFS= read -r line; do
  IFS="$us"
  # shellcheck disable=SC2086 # se trocea a propósito en las palabras que separa US
  set -- $line
  unset IFS
  segment "$@"
done <<EOF
$segments
EOF

[ "$publishes" = 1 ] || exit 0
# shellcheck disable=SC2086 # una línea por tag
tags="$(printf '%s\n' $tags | sort -u | tr '\n' ' ')"
tags="${tags% }"
case "$tags" in
  '') what="puede publicar tags v*" ;;
  *' '*) what="publica los tags $tags" ;;
  *) what="publica el tag $tags" ;;
esac
ask "Esta orden $what: subir un tag v* dispara release.yml y la release es irreversible. Confírmalo solo si quieres publicarla."
