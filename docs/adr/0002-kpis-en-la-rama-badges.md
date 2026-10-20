# ADR-0002: KPIs como JSON en una rama huérfana `badges`

- Estado: aceptado
- Fecha: 2026-10-20
- Spec: #8

## Contexto

El README quiere badges con KPIs del proyecto: cobertura, número de tests y vulnerabilidades conocidas. GitHub solo da badges de estado de workflow. Sin SaaS (ADR-0001), los KPIs tienen que publicarse en algún sitio que shields.io pueda leer, y el comentario de cobertura de cada PR necesita la cobertura de `main` para calcular la diferencia.

Alternativas descartadas: un Gist (otro token con permisos fuera del repo), GitHub Pages (un despliegue para tres JSON) y commitear los JSON en `main` (un commit por push y conflictos con el ruleset).

## Decisión

- Los KPIs se guardan en una **rama huérfana `badges`**, un JSON por KPI en formato de *endpoint* de shields.io (`schemaVersion`, `label`, `message`, `color`).
- En cada push a `main`, el CI la actualiza con el `GITHUB_TOKEN` (`contents: write` solo en ese job) en un único commit que reemplaza al anterior, para no acumular historial.
- La rama queda fuera del ruleset de `main`.
- El README los muestra con el badge *endpoint* de shields.io sobre `raw.githubusercontent.com/DavidJuanes/gadis-cli/badges/<kpi>.json`. El job `test` lee de ahí la cobertura de `main` para el comentario del PR.

## Consecuencias

- Nada sale del repo: los datos los escribe el propio CI y se pueden auditar.
- La rama `badges` se reescribe con force-push en cada actualización; no sirve como histórico de KPIs.
- `raw.githubusercontent.com` cachea unos minutos, así que un badge puede ir algo por detrás de `main`.
