# ADR-0001: Herramientas nativas de GitHub y actions oficiales fijadas por SHA, sin SaaS

- Estado: aceptado
- Fecha: 2026-10-20
- Spec: #8

## Contexto

`gadis-cli` es público y publica binarios que se instalan con `curl … | sh`. Necesita tests con informe, lint, análisis de vulnerabilidades y de secretos, compilación cruzada, CodeQL, dependencias al día y releases verificables. Para casi todo eso hay dos caminos: servicios SaaS (Codecov, SonarCloud, Snyk, Go Report Card) y actions de la comunidad del Marketplace, o lo que ya ofrece GitHub más las herramientas de cada ecosistema.

Cada SaaS es otra cuenta, otro token con acceso al repo y otro sitio donde mirar. Cada action de la comunidad es código de un tercero que se ejecuta en el CI con el `GITHUB_TOKEN` y, si se referencia por etiqueta, puede cambiar sin que nos enteremos.

## Decisión

- Se usan las **herramientas nativas de GitHub** siempre que se pueda: Actions, CodeQL, Dependabot, secret scanning con push protection, artifact attestations y rulesets. **Ningún servicio SaaS.**
- **Actions permitidas**: solo las oficiales de GitHub (`actions/*`, `github/codeql-action`) y las oficiales de cada herramienta (`golangci/golangci-lint-action`, `ossf/scorecard-action`, `goreleaser/goreleaser-action`). Lo que haría una action de la comunidad (informes, comentarios en el PR, avisos) se hace con scripts versionados en `scripts/ci/` y `gh`. Las herramientas sin action oficial se ejecutan desde su binario o módulo oficial con versión fija: gitleaks se descarga de su release y se verifica con un SHA-256 fijado en el workflow; `actionlint` y `govulncheck` se ejecutan con `go run módulo@versión`, que verifica el módulo contra `sum.golang.org`.
- **Excepciones** a esa lista, con su motivo:
  - `dependabot/fetch-metadata` (en `dependabot-automerge.yml`): no es de `actions/*` ni de una herramienta, pero la spec la nombra para leer el tipo de actualización de Dependabot, y la mantiene GitHub.
  - `actions/attest` en vez de `actions/attest-sbom`, que la spec nombra para las attestations de SBOM: GitHub ha deprecado `actions/attest-sbom` en favor de `actions/attest`, que hace lo mismo con `sbom-path`. Ambas son oficiales.
- Todas las actions se **fijan por el SHA completo del commit**, con la versión en un comentario (`uses: actions/checkout@<sha> # v7.0.1`). Dependabot las actualiza.
- Cada workflow declara `permissions: contents: read` a nivel de workflow y amplía permisos solo en el job que los necesita. `actions/checkout` va con `persist-credentials: false`.

## Consecuencias

- Todo lo que comprueba el CI se puede ejecutar en local con las mismas herramientas (`golangci-lint run`, `govulncheck ./...`, `gitleaks git`).
- Una action comprometida o una etiqueta movida no cambia lo que ejecuta el CI hasta que alguien revise y mergee el PR de Dependabot que mueve el SHA.
- Los informes de tests, la cobertura y los badges cuestan scripts propios que hay que mantener y probar (ver ADR-0002).
- Se renuncia a las vistas históricas que dan Codecov o SonarCloud.
