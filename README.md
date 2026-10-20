# gadis-cli

[![CI](https://github.com/DavidJuanes/gadis-cli/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/DavidJuanes/gadis-cli/actions/workflows/ci.yml?query=branch%3Amain)
[![CodeQL](https://github.com/DavidJuanes/gadis-cli/actions/workflows/codeql.yml/badge.svg)](https://github.com/DavidJuanes/gadis-cli/actions/workflows/codeql.yml)
[![Integración](https://github.com/DavidJuanes/gadis-cli/actions/workflows/integration.yml/badge.svg)](https://github.com/DavidJuanes/gadis-cli/actions/workflows/integration.yml)
[![Cobertura](https://img.shields.io/endpoint?url=https%3A%2F%2Fraw.githubusercontent.com%2FDavidJuanes%2Fgadis-cli%2Fbadges%2Fcoverage.json)](https://github.com/DavidJuanes/gadis-cli/actions/workflows/ci.yml?query=branch%3Amain)
[![Tests](https://img.shields.io/endpoint?url=https%3A%2F%2Fraw.githubusercontent.com%2FDavidJuanes%2Fgadis-cli%2Fbadges%2Ftests.json)](https://github.com/DavidJuanes/gadis-cli/actions/workflows/ci.yml?query=branch%3Amain)
[![Vulnerabilidades](https://img.shields.io/endpoint?url=https%3A%2F%2Fraw.githubusercontent.com%2FDavidJuanes%2Fgadis-cli%2Fbadges%2Fvulns.json)](https://github.com/DavidJuanes/gadis-cli/actions/workflows/ci.yml?query=branch%3Amain)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/DavidJuanes/gadis-cli/badge)](https://scorecard.dev/viewer/?uri=github.com/DavidJuanes/gadis-cli)
[![Release](https://img.shields.io/github/v/release/DavidJuanes/gadis-cli)](https://github.com/DavidJuanes/gadis-cli/releases/latest)
[![Go](https://img.shields.io/github/go-mod/go-version/DavidJuanes/gadis-cli)](go.mod)
[![Licencia](https://img.shields.io/github/license/DavidJuanes/gadis-cli)](LICENSE)
[![Go Reference](https://pkg.go.dev/badge/github.com/DavidJuanes/gadis-cli.svg)](https://pkg.go.dev/github.com/DavidJuanes/gadis-cli)

CLI para hacer la compra en [Gadisline](https://www.gadisline.com), la tienda online de los supermercados Gadis: buscar productos, ver categorías y gestionar el carrito de tu cuenta desde la terminal o desde scripts, LLMs y otras automatizaciones.

> Proyecto no oficial, sin relación con Gadisa Retail. Usa la API privada de la web, que puede cambiar sin aviso. Cómo funciona por dentro: [docs/api.md](docs/api.md).

## Instalación

```bash
curl -fsSL https://raw.githubusercontent.com/DavidJuanes/gadis-cli/main/install.sh | sh
```

Instala el binario `gadis` en `~/.local/bin` (cámbialo con `GADIS_INSTALL_DIR`). También puedes bajarlo de [Releases](https://github.com/DavidJuanes/gadis-cli/releases) o compilarlo:

```bash
go install github.com/DavidJuanes/gadis-cli/cmd/gadis@latest
```

## Uso

```bash
gadis login                      # inicia sesión (pide email y contraseña)
gadis whoami                     # comprueba la sesión y tu CP de reparto

gadis search "leche semidesnatada"
gadis categories --depth 3
gadis category aceite-oliva
gadis product <id>

gadis cart                       # ver el carrito
gadis cart add <id> --qty 2      # suma 2 a lo que haya
gadis cart set <id> 1            # fija la cantidad (0 lo quita)
gadis cart remove <id>
gadis cart clear --yes
```

- Todos los comandos aceptan `--json`.
- Los productos se identifican por su id (UUID), que muestran `search`, `category` y `product`.
- Los productos al peso (fruta, carne…) se piden en kilos: `gadis cart add <id> --qty 1,5`.
- El pedido se termina en la web: el CLI todavía no hace el checkout.

### Sesión

`gadis login` guarda la sesión (cookies, nunca la contraseña) en `~/.gadis/session.json` con permisos 0600. Dura unos 30 días; después, repite el login.

Credenciales, por orden de preferencia:

1. `--user` y `--password`. Ojo: la contraseña queda en el historial de la shell y es visible en `ps` mientras se ejecuta.
2. Las variables de entorno `GADIS_EMAIL` y `GADIS_PASSWORD`.
3. La terminal (la contraseña no se muestra).

Si Gadisline pide un código de verificación por SMS, el CLI lo pide por la terminal. Sin terminal, o si el login automático falla, usa `gadis login --browser`: abre Chromium para que inicies sesión tú, copia las cookies y lo cierra (necesita Chromium o Chrome instalado).

`gadis logout` borra la sesión.

### Código postal

El catálogo y los precios dependen de la tienda, y la tienda del código postal. Por defecto se usa el CP de reparto de tu cuenta. Para buscar con otro:

```bash
gadis search pan --postal-code 15405   # solo esta vez
gadis postal-code 15405                # por defecto, guardado en ~/.gadis/config.toml
gadis postal-code --clear              # volver al de la cuenta
```

Sin sesión ni CP, se usa la tienda por defecto de la web. El carrito usa siempre la tienda del propio carrito.

`GADIS_HOME` cambia el directorio de configuración (por defecto `~/.gadis`).

## Uso con LLMs

El repo incluye una skill para Claude Code en [`.claude/skills/gadis/`](.claude/skills/gadis/SKILL.md). Cópiala a `~/.claude/skills/` para usarla en cualquier proyecto. Enseña al agente a usar el CLI y a pedir confirmación antes de tocar el carrito.

## Desarrollo

```bash
go test ./...
go build ./cmd/gadis
```

`go test ./...` usa un Gadisline falso y nunca toca gadisline.com.

### Tests de integración

Ejecutan el CLI contra el backend real de Gadisline para detectar cambios en su API privada. Van detrás de la etiqueta `integration`, así que `go test ./...` no los ejecuta (el lint del CI sí los revisa, y `integration.yml` los ejecuta, ver abajo). En local se lanzan a mano:

```bash
go test -tags integration ./...
```

Los de catálogo (búsqueda, categorías, productos y códigos postales) no necesitan cuenta. Los de cuenta y carrito se activan con `GADIS_INTEGRATION_ACCOUNT=1` y usan una sesión que preparas tú antes; sin la variable se saltan y, con ella, fallan si no hay una sesión válida:

```bash
GADIS_HOME=~/.gadis-test gadis login                      # una vez; dura unos 30 días
GADIS_INTEGRATION_ACCOUNT=1 GADIS_TEST_HOME=~/.gadis-test go test -tags integration ./...
```

Los tests copian la sesión a un directorio temporal y no modifican `GADIS_TEST_HOME`. Hacen pocas peticiones y en serie, y no comprueban precios ni productos concretos, solo formas e invariantes. Los de cuenta y carrito dejan el carrito como estaba y nunca muestran la salida de los comandos ni los datos de la cuenta, ni siquiera al fallar: solo dicen qué aserción ha fallado.

#### En el CI

El workflow `integration.yml` corre los tests de catálogo cada día a las 03:00 UTC, sin sesión ni secretos. Los de cuenta y carrito solo se lanzan a mano, desde `main`, con la cuenta del mantenedor:

```bash
gh workflow run integration.yml --ref main                      # solo catálogo
gh workflow run integration.yml --ref main -f cuenta=true       # catálogo, cuenta y carrito
gh workflow run integration.yml --ref main -f forzar_fallo=true # prueba el aviso (combinable con cuenta)
```

Con `cuenta`, el job hace `gadis login` sobre un `GADIS_HOME` temporal con los secretos `GADIS_EMAIL` y `GADIS_PASSWORD` del Environment `integration` (restringido a la rama `main`) y corre los tests sin `-v` con `GADIS_INTEGRATION_ACCOUNT=1`. Como los logs de un repo público son públicos, el email y los datos de la cuenta se enmascaran, la salida de `gadis login` no se muestra y no se sube ningún artefacto. Dos ejecuciones nunca coinciden (`concurrency`), y un fallo abre un issue como los workflows programados.

Las releases se publican con GoReleaser al subir una etiqueta `v*`.

## Calidad y seguridad

### En cada PR

Cada PR y cada push a `main` pasan por [`ci.yml`](.github/workflows/ci.yml), con estos jobs:

| Job | Qué comprueba |
| --- | --- |
| `test` | `go test ./...` con [gotestsum](https://github.com/gotestyourself/gotestsum) y cobertura. [`scripts/ci/test-report`](scripts/ci/test-report) deja en el resumen de la ejecución la tabla de tests y la cobertura total y por paquete, anota cada test que falla en su línea del diff y, en los PR, mantiene un único comentario con la cobertura y la diferencia respecto a `main`. La cobertura deja fuera los paquetes de apoyo a los tests, que por convención de Go acaban en `test` (`internal/gadistest`, `internal/cli/clitest`, `scripts/ci/internal/ghtest`). El JUnit y el perfil de cobertura se descargan como artefacto `test-report`. No hay umbral de cobertura |
| `lint` | [golangci-lint](https://golangci-lint.run) con [`.golangci.yml`](.golangci.yml), [actionlint](https://github.com/rhysd/actionlint) sobre `.github/workflows` y que `go mod tidy` no cambie nada |
| `vuln` | [govulncheck](https://go.dev/doc/security/vuln/) sobre las dependencias que el código usa de verdad |
| `secrets` | [gitleaks](https://github.com/gitleaks/gitleaks) sobre todo el historial, con [`.gitleaks.toml`](.gitleaks.toml) |
| `build` | `goreleaser build --snapshot` para las 6 plataformas de la release |
| `install-sh` | `scripts/test-install.sh`: que `install.sh` aborta si el checksum o la attestation no cuadran |
| `repo-settings` | `scripts/test-repo-settings.sh`: que `scripts/repo-settings.sh` crea o actualiza los rulesets (`main` y tags `v*`) sin duplicarlos, que activa las *immutable releases*, que el Environment `integration` solo admite `main` y que los checks que exige son jobs que existen |
| `release-guard` | `scripts/test-release-guard.sh`: que `scripts/release-guard.sh`, el primer paso de `release.yml`, aborta si el tag no es `vX.Y.Z` estricto o si su commit no está en `main` |
| `badges` | Solo en push a `main`: [`scripts/ci/badges`](scripts/ci/badges) genera `coverage.json`, `tests.json` y `vulns.json` (formato *endpoint* de shields.io) a partir de los resultados de `test` y `vuln` y los publica en la rama huérfana `badges` en un único commit que reemplaza al anterior ([ADR-0002](docs/adr/0002-kpis-en-la-rama-badges.md)). De ahí salen los badges de KPIs y la cobertura de `main` del comentario de los PR |

Solo se usan herramientas nativas de GitHub y actions oficiales fijadas por SHA, sin servicios externos ([ADR-0001](docs/adr/0001-herramientas-nativas-de-github.md)).

Para pasar el CI a la primera, ejecuta lo mismo en local:

```bash
go test ./...
golangci-lint run                  # golangci-lint v2
go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12
go mod tidy && git diff --exit-code -- go.mod go.sum
go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
gitleaks git                       # gitleaks v8
goreleaser build --snapshot --clean
```

Un falso positivo de gitleaks se declara en `.gitleaks.toml` para esa regla y esa ruta, nunca con una excepción global. Si se filtra un secreto de verdad, se rota.

### Dependencias

Dependabot revisa cada semana los módulos de Go y las actions (`.github/dependabot.yml`). Las actualizaciones minor y patch llegan agrupadas en un PR por ecosistema y se ponen en auto-merge (squash): se mergean solas cuando pasan los checks obligatorios. Las major llegan en PRs separados y se revisan a mano.

### Análisis de seguridad programados

- **CodeQL** (`codeql.yml`) analiza el código Go en cada PR, en cada push a `main` y cada semana. Las alertas aparecen en la pestaña Security.
- **OpenSSF Scorecard** (`scorecard.yml`) mide las prácticas de la cadena de suministro en cada push a `main` y cada semana, y publica la puntuación del badge. Solo informa.
- **govulncheck diario** (`vuln-scheduled.yml`) revisa `main` cada día, para enterarse de CVE nuevos aunque nadie haga push.

Si una ejecución programada o lanzada a mano falla, el workflow reutilizable [`failure-issue.yml`](.github/workflows/failure-issue.yml), que todos llaman como su último job, ejecuta `scripts/ci/failure-issue`: abre un issue `needs-triage` con el enlace a la ejecución, o comenta el que ya esté abierto para ese workflow, y lo cierra cuando vuelve a pasar. Para probarlo, lanza `vuln-scheduled` a mano con `forzar_fallo`.

### Verificar una release

Cada release incluye `checksums.txt`, un SBOM en SPDX JSON por archivo (`<archivo>.sbom.json`, generado con syft) y attestations de procedencia y de SBOM firmadas en el CI de este repo. `install.sh` comprueba siempre el SHA-256 y, si tienes [`gh`](https://cli.github.com), también la attestation; si falla cualquiera de las dos, no instala nada. Para verificar a mano un archivo descargado:

```bash
sha256sum --ignore-missing -c checksums.txt          # en macOS: shasum -a 256 --ignore-missing -c checksums.txt
gh attestation verify gadis_X.Y.Z_linux_amd64.tar.gz --repo DavidJuanes/gadis-cli
gh attestation verify gadis_X.Y.Z_linux_amd64.tar.gz --repo DavidJuanes/gadis-cli \
  --predicate-type https://spdx.dev/Document/v2.3    # attestation del SBOM
```

### Gobierno del repo

`main` solo cambia por PR (squash, sin aprobaciones obligatorias) con los checks `test`, `lint`, `vuln`, `secrets`, `build` y `analyze` (CodeQL) en verde y sin alertas de CodeQL de severidad *high* o superior. Tiene historial lineal y no admite force-push ni borrado. Los administradores pueden saltarse el ruleset.

Los tags `v*` y las releases publicadas son inmutables. Solo un administrador puede crear un tag `v*` y nadie, tampoco un administrador, puede moverlo ni borrarlo. Como el *bypass* vale para todo un ruleset, son dos: `tags-v-creacion` (crear, con *bypass* para el admin) y `tags-v-inmutables` (actualizar y borrar, sin *bypass*). Las *immutable releases* están activadas: los archivos de una release publicada no se pueden sustituir. GoReleaser crea la release como borrador, sube todos los archivos y la publica al final, así que no choca con ellas.

Los rulesets viven en [`.github/rulesets/`](.github/rulesets/). [`scripts/repo-settings.sh`](scripts/repo-settings.sh) los crea o los actualiza (por nombre y *target*) con `gh api` y aplica el resto de ajustes: solo squash merge, borrar la rama al mergear, auto-merge, reporte privado de vulnerabilidades, secret scanning con push protection y validity checks, alertas y actualizaciones de seguridad de Dependabot, *immutable releases* y el Environment `integration` restringido a la rama `main`. Se puede ejecutar las veces que haga falta:

```bash
sh scripts/repo-settings.sh DavidJuanes/gadis-cli   # necesita gh con permisos de admin sobre el repo
```

El script no toca los secretos. Los de los tests de cuenta y carrito los añade el mantenedor a mano en el Environment `integration` (*Settings > Environments > integration*, o con `gh secret set --env integration`): `GADIS_EMAIL` y `GADIS_PASSWORD`, los de su cuenta de Gadisline.

Si se renombra un job obligatorio hay que cambiar también el ruleset; el job `repo-settings` falla si no coinciden.

Para reportar una vulnerabilidad, mira [`SECURITY.md`](SECURITY.md).

## Licencia

[MIT](LICENSE)
