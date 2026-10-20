# Andamiaje común de las pruebas de scripts (scripts/test-*.sh), en sh POSIX. Se carga con
#   . "$(dirname "$0")/testlib.sh"
# y deja: root (raíz del repo), work (directorio temporal que se borra al salir) y las funciones
# de abajo. Cada caso se llama $name y deja su salida y su código de salida en $work/$name.

root="$(cd "$(dirname "$0")/.." && pwd)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
fails=0

# capture DIR ORDEN...: ejecuta ORDEN con su salida en DIR/out y su código de salida en
# DIR/status, sin parar el script aunque falle.
capture() {
  dir="$1"
  shift
  mkdir -p "$dir"
  set +e
  "$@" >"$dir/out" 2>&1
  echo $? >"$dir/status"
  set -e
}

# fail MENSAJE: apunta un fallo del caso $name y muestra su salida.
fail() {
  echo "FAIL $name: $*"
  sed 's/^/    /' "$work/$name/out"
  fails=$((fails + 1))
}

# status: código de salida del caso $name.
status() { cat "$work/$name/status"; }

# finish SCRIPT: resume y sale con 1 si ha fallado algún caso.
finish() {
  if [ "$fails" -gt 0 ]; then
    echo "$fails fallo(s)"
    exit 1
  fi
  echo "$1: todas las pruebas pasan"
}

# isolated ORDEN...: ejecuta ORDEN sin la configuración de git del usuario ni la del sistema y con
# una identidad fija, para que el resultado no dependa de la máquina.
isolated() {
  env GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1 \
    GIT_AUTHOR_NAME=test GIT_AUTHOR_EMAIL=test@example.com \
    GIT_COMMITTER_NAME=test GIT_COMMITTER_EMAIL=test@example.com "$@"
}

# isolated_git ARG...: git aislado como en isolated.
isolated_git() { isolated git "$@"; }
