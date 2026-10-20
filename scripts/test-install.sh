#!/bin/sh
# Prueba install.sh contra una release falsa: un `curl` falso sirve los archivos desde un
# directorio local y un `gh` falso opcional registra cómo se le llama. No toca la red.
# Uso: sh scripts/test-install.sh
set -eu

. "$(dirname "$0")/testlib.sh"

case "$(uname -s)" in Linux) os=linux ;; Darwin) os=darwin ;; esac
case "$(uname -m)" in x86_64 | amd64) arch=amd64 ;; arm64 | aarch64) arch=arm64 ;; esac
version=9.9.9
archive="gadis_${version}_${os}_${arch}.tar.gz"

# Release falsa: un archivo con un script `gadis` y su checksums.txt (bueno y alterado).
release="$work/release"
mkdir -p "$release/src"
printf '#!/bin/sh\necho fake gadis\n' >"$release/src/gadis"
chmod +x "$release/src/gadis"
tar -czf "$release/$archive" -C "$release/src" gadis
if command -v sha256sum >/dev/null 2>&1; then
  sum="$(sha256sum "$release/$archive" | cut -d' ' -f1)"
else
  sum="$(shasum -a 256 "$release/$archive" | cut -d' ' -f1)"
fi
printf '%s  %s\n' "$sum" "$archive" >"$release/checksums.txt.good"
printf '%s  %s\n' 0000000000000000000000000000000000000000000000000000000000000000 "$archive" >"$release/checksums.txt.bad"

# PATH mínimo con lo que necesita install.sh, para que nunca se vea un `gh` real.
base="$work/bin"
mkdir -p "$base"
for t in sh sed grep cut head sha256sum shasum tar gzip mkdir install mktemp rm uname cat; do
  p="$(command -v "$t" 2>/dev/null || true)"
  case "$p" in /*) ln -s "$p" "$base/$t" ;; esac
done
cat >"$base/curl" <<CURL
#!/bin/sh
# curl falso: sirve el último segmento de la URL desde la release falsa.
out=""; url=""
while [ \$# -gt 0 ]; do
  case "\$1" in -o) out="\$2"; shift ;; -*) ;; *) url="\$1" ;; esac
  shift
done
case "\$url" in
  */releases/latest) echo '{"tag_name": "v$version"}'; exit 0 ;;
esac
name="\${url##*/}"
[ "\$name" = checksums.txt ] && name="checksums.txt.\$FAKE_CHECKSUMS"
[ -f "$release/\$name" ] || exit 22
cat "$release/\$name" >"\$out"
CURL
chmod +x "$base/curl"

# gh falso: registra sus argumentos y sale con $FAKE_GH_EXIT.
ghbin="$work/ghbin"
mkdir -p "$ghbin"
cat >"$ghbin/gh" <<'GH'
#!/bin/sh
echo "$*" >>"$FAKE_GH_LOG"
exit "${FAKE_GH_EXIT:-0}"
GH
chmod +x "$ghbin/gh"

# run CASO CHECKSUMS(good|bad) GH(none|ok|fail): ejecuta install.sh y deja salida y estado en $work/CASO.
run() {
  dir="$work/$1"
  path="$base"
  [ "$3" = none ] || path="$ghbin:$base"
  gh_exit=0
  [ "$3" = fail ] && gh_exit=1
  capture "$dir" env -i HOME="$dir" PATH="$path" GADIS_INSTALL_DIR="$dir/install" \
    FAKE_CHECKSUMS="$2" FAKE_GH_EXIT="$gh_exit" FAKE_GH_LOG="$dir/gh.log" \
    sh "$root/install.sh"
}

installed() { [ -x "$work/$name/install/gadis" ]; }
output_has() { grep -q "$1" "$work/$name/out"; }

name=checksum_ok_without_gh
run "$name" good none
[ "$(status)" = 0 ] || fail "sale con $(status), se esperaba 0"
installed || fail "no instala gadis"
output_has "attestation" || fail "no avisa de que omite la verificación de la attestation"

name=checksum_altered
run "$name" bad none
[ "$(status)" != 0 ] || fail "sale con 0, se esperaba un error"
! installed || fail "instala gadis con un checksum alterado"
output_has "SHA-256" || fail "no informa del error de checksum"

name=checksum_altered_with_gh
run "$name" bad ok
[ "$(status)" != 0 ] || fail "sale con 0, se esperaba un error"
! installed || fail "instala gadis con un checksum alterado"

name=attestation_ok
run "$name" good ok
[ "$(status)" = 0 ] || fail "sale con $(status), se esperaba 0"
installed || fail "no instala gadis"
grep -q "^attestation verify .*/$archive --repo DavidJuanes/gadis-cli\$" "$work/$name/gh.log" 2>/dev/null ||
  fail "no llama a gh attestation verify sobre el archivo: $(cat "$work/$name/gh.log" 2>/dev/null || true)"

name=attestation_fails
run "$name" good fail
[ "$(status)" != 0 ] || fail "sale con 0, se esperaba un error"
! installed || fail "instala gadis con una attestation que no verifica"

finish install.sh
