# ADR-0003: Releases con un tag manual e inmutable; si fallan, el número se quema

- Estado: aceptado
- Fecha: 2026-10-20
- Spec: #20

## Contexto

`release.yml` publica una release al subir un tag `v*`, y sus attestations de Sigstore prometen que cada archivo sale del commit de ese tag. Hasta ahora cualquier tag `v*` desde cualquier rama publicaba, y un tag se podía mover o borrar después, lo que rompe esa promesa: el tag dejaría de apuntar al código que se attestó.

Para lanzar una release había dos caminos: automatizarla (release-please, semantic-release o un `workflow_dispatch` que calcule la versión y cree el tag) o subir el tag a mano. Y ante una release fallida, otros dos: reutilizar el número (borrar el tag y la release y volver a subirlos) o pasar al siguiente.

## Decisión

- **Tag manual**: el mantenedor (o Claude, con su confirmación explícita) sube un tag anotado `vX.Y.Z`, sin firmar, sobre un commit de `main`. Sin servicios ni actions de terceros (ADR-0001). La versión se calcula de forma mecánica a partir de las etiquetas de los PR, pero la decisión de publicar es humana. La procedencia la dan las attestations y el ruleset, no la firma del tag.
- **Guarda en `release.yml`**: su primer paso aborta si el tag no es `vX.Y.Z` estricto o si su commit no es alcanzable desde `main`.
- **Tags inmutables**: dos rulesets sobre `refs/tags/v*`, porque el *bypass* vale para todo un ruleset. `tags-v-creacion` impide crear tags con *bypass* para el admin; `tags-v-inmutables` impide actualizarlos y borrarlos, sin *bypass*, tampoco para el admin.
- **Releases inmutables**: las *immutable releases* están activadas, así que una release publicada ya no admite archivos nuevos. GoReleaser crea la release como borrador, sube todos los archivos y la publica al final, así que no choca con ellas y no hay que tocar su opción `draft`.
- **Número quemado**: si una release falla, no se reintenta con el mismo número. Se corrige en un PR y se publica el siguiente *patch*. Una release incompleta se borra; el tag se queda.
- **Confirmación con Claude**: un hook `PreToolUse` versionado (`scripts/confirm-tag-push.sh`) pide confirmación ante cualquier orden que publique un tag. Devuelve «ask», no «deny».

## Consecuencias

- Un tag `v*` siempre apunta al commit que se attestó, y los archivos de una release publicada son los que se verificaron.
- Un error al etiquetar no se deshace: un tag mal formado o fuera de `main` se queda en el repo sin release, y una release fallida deja un hueco en la numeración.
- Publicar cuesta unos pasos a mano (comprobar el CI, lanzar la integración, calcular la versión), recogidos en `docs/releasing.md` y que Claude puede hacer por el mantenedor.
- Las attestations se guardan aparte de la release: si falla su job, la release queda publicada sin ellas y hay que borrarla y pasar al siguiente *patch*.
