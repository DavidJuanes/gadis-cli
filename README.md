# gadis-cli

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

Las releases se publican con GoReleaser al subir una etiqueta `v*`.

## Licencia

[MIT](LICENSE)
