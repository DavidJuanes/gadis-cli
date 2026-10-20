# Política de seguridad

## Versiones mantenidas

Solo se corrigen fallos de seguridad en la última release publicada. Si usas una versión anterior, actualiza antes de reportar (`install.sh` instala siempre la última).

| Versión | Mantenida |
| --- | --- |
| Última release | Sí |
| Anteriores | No |

## Cómo reportar una vulnerabilidad

No abras un issue público. Repórtala en privado desde la pestaña **Security** del repo, con **Report a vulnerability** ([enlace directo](https://github.com/DavidJuanes/gadis-cli/security/advisories/new)).

Incluye, si puedes:

- Qué versión de `gadis` usas y en qué sistema.
- Cómo reproducir el fallo y qué impacto tiene (por ejemplo, exposición de la sesión o de las credenciales de Gadisline).
- Si se te ocurre, una posible corrección.

Es un proyecto personal mantenido en el tiempo libre: intentaré contestar en una semana. Cuando haya una corrección, se publicará una release y un aviso de seguridad, con crédito para quien lo reportó si así lo quiere.

## Fuera de alcance

`gadis` usa la API privada de Gadisline. Los fallos de la propia web o la API de Gadisline se reportan a Gadis, no aquí.
