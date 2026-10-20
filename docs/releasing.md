# Publicar una release

Las releases salen bajo demanda, con un tag anotado `vX.Y.Z` sobre un commit de `main`. Subir el tag dispara [`release.yml`](../.github/workflows/release.yml), que compila con GoReleaser, publica la release y firma las attestations. Una release publicada es irreversible: el tag no se puede mover ni borrar y sus archivos no se pueden sustituir. El porqué está en el [ADR-0003](adr/0003-tag-manual-inmutable.md).

En los ejemplos, `SHA` es el commit candidato de `main` y `vX.Y.Z` la versión que se publica.

## 0. Ajustes del repo

[`scripts/repo-settings.sh`](../scripts/repo-settings.sh) tiene que estar aplicado: crea los rulesets de tags `tags-v-creacion` (solo el admin crea tags `v*`) y `tags-v-inmutables` (nadie los mueve ni los borra), activa las *immutable releases* y crea las etiquetas `breaking` y `skip-changelog`. Compruébalo:

```bash
gh api repos/DavidJuanes/gadis-cli/immutable-releases --jq .enabled              # true
gh api 'repos/DavidJuanes/gadis-cli/rulesets?targets=tag' --jq '.[].name'        # tags-v-creacion y tags-v-inmutables
```

Si falta algo, aplícalo antes de seguir: `sh scripts/repo-settings.sh DavidJuanes/gadis-cli`.

## 1. Condiciones previas

El commit candidato es la cabeza de `main`:

```bash
git fetch origin main && git rev-parse origin/main
```

**`ci.yml` en verde en ese commit** (la ejecución del push a `main`):

```bash
gh run list --workflow ci.yml --commit SHA --event push --json status,conclusion
```

**`integration.yml` con `cuenta` en verde en ese commit.** Se lanza a mano y corre sobre la cabeza de `main` en ese momento:

```bash
gh workflow run integration.yml --ref main -f cuenta=true
gh run list --workflow integration.yml --event workflow_dispatch --limit 1 --json databaseId,headSha
gh run watch ID --exit-status
```

El `headSha` de la ejecución tiene que ser `SHA`. Si `main` ha avanzado entre medias, el candidato pasa a ser el commit nuevo y se repiten las dos comprobaciones.

## 2. Versión

SemVer desde `v0.1.0`. El contrato público son los nombres de los comandos y subcomandos, los flags, el formato de la salida JSON y los códigos de salida: romper cualquiera es un cambio incompatible (etiqueta `breaking`). `v1.0.0` sale cuando el mantenedor declare estable el contrato.

La versión sale de las etiquetas de los PR fusionados en `main` desde el último tag:

```bash
git describe --tags --abbrev=0 origin/main                    # último tag; sin tags, falla
git log -1 --format=%cI vA.B.C                                # su fecha
gh pr list --state merged --base main --search "merged:>FECHA" --limit 500 \
  --json number,title,labels,mergedAt
```

Sin tags, cuentan todos los PR fusionados y la versión es **`v0.1.0`**.

En 0.x, sobre el último tag `v0.Y.Z`:

| PR desde el último tag | Siguiente versión |
| --- | --- |
| Alguno con `breaking` o `enhancement` | *minor*: `v0.(Y+1).0` |
| Solo `bug`, `dependencies`, `documentation`, `skip-changelog` u otras | *patch*: `v0.Y.(Z+1)` |

Desde `v1.0.0` rige SemVer normal: `breaking` sube el *major*, `enhancement` el *minor* y el resto el *patch*.

Si no hay ningún PR desde el último tag, no hay release.

### Borrador de las notas

GitHub genera las notas a partir de los PR, agrupados según [`.github/release.yml`](../.github/release.yml): ⚠️ Cambios incompatibles (`breaking`), Novedades (`enhancement`), Correcciones (`bug`), Dependencias (`dependencies`) y Otros. Los PR con `skip-changelog` no salen. Para verlas antes de publicar (no crea nada):

```bash
gh api repos/DavidJuanes/gadis-cli/releases/generate-notes \
  -f tag_name=vX.Y.Z -f target_commitish=SHA [-f previous_tag_name=vA.B.C] --jq .body
```

Un PR sin etiqueta cae en Otros y no sube el *minor*. Si alguno está mal etiquetado, corrígelo en el PR (`gh pr edit N --add-label …`) antes de calcular la versión.

## 3. Tag

Anotado, sin firmar, sobre `SHA`:

```bash
git tag -a vX.Y.Z -m vX.Y.Z SHA
git push origin vX.Y.Z
```

Con Claude, el hook [`scripts/confirm-tag-push.sh`](../scripts/confirm-tag-push.sh) pide confirmación antes del `git push` del tag (y de cualquier otra orden que publique uno).

El primer paso de `release.yml` es [`scripts/release-guard.sh`](../scripts/release-guard.sh): aborta, sin publicar nada, si el tag no es `vX.Y.Z` estricto (sin ceros a la izquierda ni sufijos: `v1.0`, `v01.2.3` o `v0.2.0-rc.1` no valen) o si su commit no es alcanzable desde `main`.

## 4. Vigilar `release.yml`

```bash
gh run list --workflow release.yml --branch vX.Y.Z --json databaseId,status,conclusion
gh run watch ID --exit-status
```

Tiene dos jobs: `goreleaser` (guarda, `go test`, GoReleaser y attestation de procedencia) y `sbom` (una attestation de SBOM por archivo). La release solo está completa si los dos acaban en verde.

## 5. Verificación posterior

**Archivos de la release**: 13 en total, publicada e inmutable.

```bash
gh release view vX.Y.Z --json isDraft,isImmutable,assets --jq '{isDraft, isImmutable, assets: [.assets[].name]}'
```

- 6 archivos `gadis_X.Y.Z_{linux,darwin}_{amd64,arm64}.tar.gz` y `gadis_X.Y.Z_windows_{amd64,arm64}.zip`.
- `checksums.txt`.
- 6 SBOM `<archivo>.sbom.json`.

**`install.sh` instala esa versión** verificando el SHA-256 y la attestation (necesita `gh` con sesión iniciada; sin `gh` solo comprueba el SHA-256):

```bash
dir="$(mktemp -d)"
curl -fsSL https://raw.githubusercontent.com/DavidJuanes/gadis-cli/main/install.sh |
  GADIS_VERSION=vX.Y.Z GADIS_INSTALL_DIR="$dir" sh
"$dir/gadis" --version                                        # gadis version X.Y.Z
```

`GADIS_VERSION` lleva la `v`; `gadis --version` la muestra sin ella.

**La attestation del SBOM** de un archivo:

```bash
gh release download vX.Y.Z -p gadis_X.Y.Z_linux_amd64.tar.gz -D "$dir"
gh attestation verify "$dir/gadis_X.Y.Z_linux_amd64.tar.gz" --repo DavidJuanes/gadis-cli \
  --predicate-type https://spdx.dev/Document/v2.3
```

**`go install` con el tag**:

```bash
GOBIN="$(mktemp -d)" go install github.com/DavidJuanes/gadis-cli/cmd/gadis@vX.Y.Z
```

Tiene que compilar. El binario de `go install` dice `gadis version dev`: la versión solo la inyecta GoReleaser con `-ldflags`.

## Si la release falla

El número se quema: el tag ya no se puede mover ni borrar. Se corrige en un PR y se publica el siguiente *patch* repitiendo la guía.

- **La guarda aborta** (tag mal formado o fuera de `main`): no se ha publicado nada. El tag se queda en el repo sin release.
- **Falla antes de publicar** (`go test`, syft o el *preflight* de GoReleaser): no hay release. Igual que el caso anterior.
- **GoReleaser se corta subiendo archivos**: queda una release en borrador, que no es pública ni «latest». Bórrala: `gh release delete vX.Y.Z --yes` (sin `--cleanup-tag`: el ruleset no deja borrar el tag).
- **La release se ha publicado pero falla la attestation de procedencia o el job `sbom`**: la release está incompleta y es «latest», así que `install.sh` con `gh` no la puede verificar. Bórrala con `gh release delete vX.Y.Z --yes` y publica el siguiente *patch* cuanto antes.

## *Patch* de seguridad

Cuando se fusiona en `main` la corrección de una vulnerabilidad que afecta al binario publicado (en el código o en una dependencia que señale `govulncheck`), se publica una release en ese momento, sin esperar a juntar más cambios. Es la que promete [`SECURITY.md`](../SECURITY.md): solo se mantiene la última release. El número sigue las reglas del paso 2. Después se publica el aviso de seguridad del repo.
