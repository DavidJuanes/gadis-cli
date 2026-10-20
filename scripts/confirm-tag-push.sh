#!/bin/sh
# Hook PreToolUse de Claude Code (registrado en .claude/settings.json con matcher Bash): pide
# confirmación antes de cualquier orden que publique, mueva o borre un tag, porque subir un tag
# v* dispara release.yml y la release es irreversible. Lee de stdin el JSON de PreToolUse y mira
# tool_input.command. Si la orden toca un tag, escribe un JSON con permissionDecision «ask» y un
# motivo que dice qué hace y nombra el tag; si no, no escribe nada. Siempre sale con 0. Ante la
# duda, pregunta. Se prueba con scripts/test-confirm-tag-push.sh.
# Uso: sh scripts/confirm-tag-push.sh <JSON_DE_PRETOOLUSE
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
# vuelven a partir como órdenes, por si son un sh -c "git push ...". Cada comilla invertida se
# convierte antes en $`, para que la palabra que la precede acabe en $ igual que ante un $(...):
# así se ve que el argumento solo se conoce al ejecutar. Basta para reconocer las palabras; no
# es un intérprete de sh y no ejecuta nada.
word_sep="$(printf '\037')"
segments="$(printf '%s' "$input" | jq -r '
  def words: [scan("(?:[^\\s;&|()`\"'"'"']|\"(?:[^\"\\\\]|\\\\.)*\"|'"'"'[^'"'"']*'"'"')+|[;&|()`\\n]")]
    | reduce .[] as $w ([[]];
        if ($w | test("^[;&|()`\\n]$")) then . + [[]]
        else .[-1] += [$w | gsub("[\"'"'"'\\\\]"; "")] end)
    | .[] | select(length > 0) | join("\u001f");
  (.tool_input.command // "" | gsub("`"; "$`")) as $c
  | ($c | words),
    ($c | scan("\"(?:[^\"\\\\]|\\\\.)*\"|'"'"'[^'"'"']*'"'"'") | .[1:-1] | words)
')"
# Directorio en el que corre la orden; un cd dentro de la cadena lo cambia.
dir="$(printf '%s' "$input" | jq -r '.cwd // ""')"
[ -n "$dir" ] || dir="$PWD"

# Lo que hace la orden con los tags: una línea «VERBO [TAG]» por cada uno, con VERBO publica,
# mueve o borra. Sin TAG, la orden hace eso con algún tag que no se conoce.
entries=""

# found VERBO [TAG]: apunta que la orden hace VERBO con TAG (o con algún tag, si no se da).
found() {
  tag="${2:-}"
  entries="$entries
$1 ${tag#refs/tags/}"
}

# is_tag REF: si REF es un tag en un refspec. Solo cuentan v[0-9]* y refs/tags/*, para que ramas
# como vendor-update no pregunten.
is_tag() {
  case "$1" in v[0-9]* | refs/tags/*) return 0 ;; *) return 1 ;; esac
}

# git_push ARG...: analiza los argumentos de un git push que corre en $gitdir con las opciones
# -c de $gitconfig.
git_push() {
  follow=""
  deletes=0
  while [ $# -gt 0 ]; do
    case "$1" in
      --tags | --follow-tags | --mirror) found publica ;;
      --no-follow-tags) follow=false ;;
      -d | --delete) deletes=1 ;;
      # Opciones que llevan valor en la palabra siguiente: el valor no es un refspec.
      -o | --push-option | --repo | --receive-pack | --exec) [ $# -lt 2 ] || shift ;;
      -*) ;;
      # Un argumento que solo se conoce al ejecutar ($VAR, $(...), `...`) puede ser un tag.
      *'$'*) found publica ;;
      *)
        # Refspec [+]ORIGEN[:DESTINO]: cuenta si alguno de los lados es un tag. Con --delete o
        # sin ORIGEN (:DESTINO), lo borra.
        source="${1%%:*}"
        verb=publica
        if [ "$deletes" = 1 ] || { [ -z "${source#+}" ] && [ "$source" != "$1" ]; }; then
          verb=borra
        fi
        for side in "$source" "${1#*:}"; do
          side="${side#+}"
          ! is_tag "$side" || found "$verb" "$side"
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
  [ "$follow" != true ] || found publica
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
        found publica "$1"
        return
        ;;
    esac
    shift
  done
  found publica
}

# gh_api ARG...: cuenta si escribe (método distinto de GET, o campos sin método) sobre git/refs,
# git/tags o una release con tag_name (que crea el tag si no existe), o si es la mutación
# createRef de GraphQL. El tag sale de la ruta (git/refs/tags/TAG) o de los campos ref=, tag= y
# tag_name=.
gh_api() {
  method=""
  fields=0
  input=0
  path=""
  named=""
  release_tags=""
  creates_ref=0
  while [ $# -gt 0 ]; do
    case "$1" in
      -X | --method) [ $# -lt 2 ] || { method="$2"; shift; } ;;
      -X* | --method=*) method="${1#-X}" method="${method#--method=}" ;;
      -f | -F | --field | --raw-field)
        fields=1
        [ $# -lt 2 ] || {
          case "$2" in
            ref=refs/tags/* | tag=*) named="$named ${2#*=}" ;;
            tag_name=*) release_tags="$release_tags ${2#*=}" ;;
            *createRef*) creates_ref=1 ;;
          esac
          shift
        }
        ;;
      --input) input=1 fields=1 && { [ $# -lt 2 ] || shift; } ;;
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
    graphql)
      [ "$creates_ref" = 0 ] || found publica
      ;;
    */git/refs/tags/* | git/refs/tags/*)
      case "$method" in DELETE) verb=borra ;; PATCH) verb=mueve ;; *) verb=publica ;; esac
      found "$verb" "${path##*git/refs/tags/}"
      ;;
    *git/refs* | *git/tags*)
      for tag in $named; do found publica "$tag"; done
      found publica
      ;;
    */releases | */releases/[0-9]*)
      for tag in $release_tags; do found publica "$tag"; done
      [ "$input" = 0 ] || found publica
      ;;
  esac
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
  # Con xargs, los argumentos del git push llegan de la entrada al ejecutar.
  via_xargs=0
  while [ $# -gt 0 ]; do
    case "$1" in
      xargs) via_xargs=1 ;;
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
          [ "$via_xargs" = 0 ] || found publica
          git_push "$@"
          return
        fi
        ;;
      gh)
        shift
        # Opciones globales de gh antes de la suborden (gh -R OWNER/REPO release ...).
        while [ $# -gt 0 ]; do
          case "$1" in
            -R | --repo) [ $# -lt 2 ] || shift ;;
            -*) ;;
            *) break ;;
          esac
          shift
        done
        if [ "${1:-}" = release ] && [ "${2:-}" = create ]; then
          shift 2
          gh_release_create "$@"
          return
        elif [ "${1:-}" = api ]; then
          shift
          gh_api "$@"
          return
        fi
        continue
        ;;
    esac
    [ $# -eq 0 ] || shift
  done
}

set -f
while IFS= read -r line; do
  IFS="$word_sep"
  # shellcheck disable=SC2086 # se trocea a propósito en las palabras que separa US
  set -- $line
  unset IFS
  segment "$@"
done <<EOF
$segments
EOF

[ -n "$entries" ] || exit 0

# Una frase por verbo, con sus tags («publica el tag v0.1.0», «borra los tags v0.1.0 v0.1.1») o,
# si no se conoce ninguno, en general («puede publicar tags v*»).
what=""
for verb in publica mueve borra; do
  printf '%s\n' "$entries" | grep -q "^$verb" || continue
  verb_tags="$(printf '%s\n' "$entries" | sed -n "s/^$verb \(..*\)/\1/p" | sort -u | tr '\n' ' ')"
  verb_tags="${verb_tags% }"
  case "$verb_tags" in
    '')
      case "$verb" in publica) infinitive=publicar ;; mueve) infinitive=mover ;; borra) infinitive=borrar ;; esac
      phrase="puede $infinitive tags v*"
      ;;
    *' '*) phrase="$verb los tags $verb_tags" ;;
    *) phrase="$verb el tag $verb_tags" ;;
  esac
  what="${what:+$what y }$phrase"
done
if ! printf '%s\n' "$entries" | grep -Eq '^(mueve|borra)'; then
  ask "Esta orden $what: subir un tag v* dispara release.yml y la release es irreversible. Confírmalo solo si quieres publicarla."
fi
ask "Esta orden $what: los tags v* no se pueden mover ni borrar en GitHub y su release es irreversible. Confírmalo solo si quieres cambiarlos."
