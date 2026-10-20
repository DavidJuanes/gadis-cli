# gadis-cli

[![CodeQL](https://github.com/DavidJuanes/gadis-cli/actions/workflows/codeql.yml/badge.svg)](https://github.com/DavidJuanes/gadis-cli/actions/workflows/codeql.yml)
[![Integración](https://github.com/DavidJuanes/gadis-cli/actions/workflows/integration.yml/badge.svg)](https://github.com/DavidJuanes/gadis-cli/actions/workflows/integration.yml)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/DavidJuanes/gadis-cli/badge)](https://scorecard.dev/viewer/?uri=github.com/DavidJuanes/gadis-cli)

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

Ejecutan el CLI contra el backend real de Gadisline para detectar cambios en su API privada. Van detrás de la etiqueta `integration`, así que ni `go test ./...` ni el CI de cada PR los compilan. En local se lanzan a mano:

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

### Dependencias

Dependabot revisa cada semana los módulos de Go y las actions (`.github/dependabot.yml`). Las actualizaciones minor y patch llegan agrupadas en un PR por ecosistema y se ponen en auto-merge (squash): se mergean solas cuando pasan los checks obligatorios. Las major llegan en PRs separados y se revisan a mano.

### Análisis de seguridad programados

- **CodeQL** (`codeql.yml`) analiza el código Go en cada PR, en cada push a `main` y cada semana. Las alertas aparecen en la pestaña Security.
- **OpenSSF Scorecard** (`scorecard.yml`) mide las prácticas de la cadena de suministro en cada push a `main` y cada semana, y publica la puntuación del badge. Solo informa.
- **govulncheck diario** (`vuln-scheduled.yml`) revisa `main` cada día, para enterarse de CVE nuevos aunque nadie haga push.

Si una ejecución programada o lanzada a mano falla, `scripts/ci/failure-issue` abre un issue `needs-triage` con el enlace a la ejecución, o comenta el que ya esté abierto para ese workflow, y lo cierra cuando vuelve a pasar. Para probarlo, lanza `vuln-scheduled` a mano con `forzar_fallo`.

## Licencia

[MIT](LICENSE)
