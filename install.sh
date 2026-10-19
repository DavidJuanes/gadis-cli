#!/bin/sh
# Installs the latest gadis release from GitHub into ~/.local/bin (or $GADIS_INSTALL_DIR).
# Usage: curl -fsSL https://raw.githubusercontent.com/DavidJuanes/gadis-cli/main/install.sh | sh
set -eu

REPO="DavidJuanes/gadis-cli"
DIR="${GADIS_INSTALL_DIR:-$HOME/.local/bin}"

case "$(uname -s)" in
  Linux) os=linux ;;
  Darwin) os=darwin ;;
  *) echo "Sistema no soportado: $(uname -s). Descarga el binario de https://github.com/$REPO/releases" >&2; exit 1 ;;
esac
case "$(uname -m)" in
  x86_64 | amd64) arch=amd64 ;;
  arm64 | aarch64) arch=arm64 ;;
  *) echo "Arquitectura no soportada: $(uname -m)" >&2; exit 1 ;;
esac

tag="${GADIS_VERSION:-$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n1)}"
[ -n "$tag" ] || { echo "No se encontró ninguna release de $REPO" >&2; exit 1; }
version="${tag#v}"
archive="gadis_${version}_${os}_${arch}.tar.gz"
base="https://github.com/$REPO/releases/download/$tag"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
echo "Descargando gadis $tag ($os/$arch)…"
curl -fsSL -o "$tmp/$archive" "$base/$archive"
curl -fsSL -o "$tmp/checksums.txt" "$base/checksums.txt"

expected="$(grep " $archive\$" "$tmp/checksums.txt" | cut -d' ' -f1)"
if command -v sha256sum >/dev/null 2>&1; then
  actual="$(sha256sum "$tmp/$archive" | cut -d' ' -f1)"
else
  actual="$(shasum -a 256 "$tmp/$archive" | cut -d' ' -f1)"
fi
[ -n "$expected" ] && [ "$expected" = "$actual" ] || { echo "La suma SHA-256 no coincide" >&2; exit 1; }

tar -xzf "$tmp/$archive" -C "$tmp" gadis
mkdir -p "$DIR"
install -m 0755 "$tmp/gadis" "$DIR/gadis"
echo "Instalado en $DIR/gadis"
case ":$PATH:" in
  *":$DIR:"*) ;;
  *) echo "Añade $DIR a tu PATH para usar \`gadis\`." ;;
esac
