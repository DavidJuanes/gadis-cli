## Agent skills

### Issue tracker

Los issues viven en GitHub Issues de DavidJuanes/gadis-cli (con `gh`). See `docs/agents/issue-tracker.md`.

### Triage labels

Las cinco etiquetas por defecto (needs-triage, needs-info, ready-for-agent, ready-for-human, wontfix). See `docs/agents/triage-labels.md`.

### Domain docs

Contexto único: `GLOSSARY.md` y `docs/adr/` en la raíz. See `docs/agents/domain.md`.

## Etiquetas de los PR

Todo PR que abras lleva al menos una de `breaking`, `enhancement`, `bug`, `dependencies`, `documentation` o `skip-changelog` (`gh pr create --label …`). De ellas salen las notas de la release (`.github/release.yml`) y el número de versión.

## Releases

Para sacar una release sigue [`docs/releasing.md`](docs/releasing.md):

1. Comprueba los ajustes del repo y que `ci.yml` está en verde en la cabeza de `main`.
2. Lanza `integration.yml` con `cuenta=true` y espera a que acabe en verde sobre ese commit.
3. Propón la versión, justificada con las etiquetas de los PR desde el último tag, y el borrador de las notas.
4. Pide confirmación explícita al mantenedor y espera su OK.
5. Crea el tag anotado y súbelo. El hook `scripts/confirm-tag-push.sh` volverá a pedir confirmación: es irreversible.
6. Vigila `release.yml` hasta que acabe y haz la verificación posterior. Si falla, el número se quema: avisa y sigue la guía.
