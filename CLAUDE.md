## Agent skills

### Issue tracker

Los issues viven en GitHub Issues de DavidJuanes/gadis-cli (con `gh`). See `docs/agents/issue-tracker.md`.

### Triage labels

Las cinco etiquetas por defecto (needs-triage, needs-info, ready-for-agent, ready-for-human, wontfix). See `docs/agents/triage-labels.md`.

### Domain docs

Contexto único: `GLOSSARY.md` y `docs/adr/` en la raíz. See `docs/agents/domain.md`.

## Etiquetas de los PR

Todo PR que abras lleva al menos una de `breaking`, `enhancement`, `bug`, `dependencies`, `documentation` o `skip-changelog` (`gh pr create --label …`). De ellas salen las notas de la release (`.github/release.yml`) y el número de versión.
