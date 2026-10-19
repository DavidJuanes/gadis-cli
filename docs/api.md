# API de Gadisline (ingeniería inversa)

Notas de cómo funciona por dentro <https://www.gadisline.com>, la tienda online de Gadis. No es una API pública ni documentada: puede cambiar sin aviso. Todo lo de aquí se comprobó a mano el 2026-10-19.

## Visión general

- La web es una app **Next.js**. El catálogo, las tiendas y el carrito son microservicios REST detrás de un gateway Kong:
  - `https://catalog.gadisline.com/api/v3` — productos, categorías, marcas.
  - `https://store.gadisline.com/api/v3` — tiendas, códigos postales, calendario de reparto.
  - `https://cart.gadisline.com/api/v3` — carritos (requiere `Authorization: Bearer`).
  - Otros que aparecen en el JS y no se han probado: `clients.`, `order.`, `promotion.`, `masters.`, `site.`, `cms.`.
- Además, la propia web expone rutas de servidor en `https://www.gadisline.com/api/config/*`. Usan las cookies de sesión de la web y por dentro llaman a los microservicios con el token. **Son la forma de escribir en el carrito sin token**, tanto en anónimo como con sesión.
- Imágenes: `https://storage.gadisline.com/catalog/<uuid>` y `/cms/<uuid>`.

## Cabeceras obligatorias

Casi todas las llamadas a los microservicios exigen dos cabeceras. Sin ellas responden `400 Required header '…' is not present`.

| Cabecera | Valor | De dónde sale |
|---|---|---|
| `site-id` | `56df88f9-479f-4361-891e-e1864dba1ca3` | Fijo para Gadisline. Está en `__NEXT_DATA__` → `props.pageProps.site.id`. |
| `store-id` | UUID de la tienda | Depende del código postal (ver abajo). Por defecto la web usa `891d5c1e-a7a0-4287-9ea3-30c5703a4f63` (`site.default_assortment_store`). |

Recomendadas: `accept-language: es`, `Accept: application/json`, `Content-Type: application/json`, `Time-Zone: Europe/Madrid`.

## Código postal → tienda

```
GET https://store.gadisline.com/api/v3/stores/postal-codes/delivery
site-id: <site>
```

Sin cookies ni token. Devuelve `{"elements":[{"store_id","postal_code","minimum_order_quantity","minimum_shipping_free","shipping_costs"}, …]}`: unos 1.030 códigos postales repartidos entre 71 tiendas. Si un CP no aparece, no hay reparto a domicilio en esa zona.

Tiendas con recogida:

```
GET https://store.gadisline.com/api/v3/stores/public?filter_fields=pickup_store&filterclass_pickup_store=boolean&filtertype_pickup_store=eq&filtervalue_pickup_store=true
site-id: <site>
```

### CP de la cuenta

Con sesión, el CP y la tienda por defecto del cliente salen de:

```
GET https://clients.gadisline.com/api/v3/clients/<clientId>/default-delivery
Authorization: Bearer <accessToken>
site-id: <site>
```

→ `{"postal_code","store_id","delivery_type"}`. `clientId` es `__NEXT_DATA__` → `props.pageProps.clientData.id`. Direcciones guardadas: `GET …/clients/<clientId>/addresses`.

**Cuidado:** el CP del carrito de la cuenta (`pageProps.cart.postal_code`) y `pageProps.site.currentPostalCode` **no** son el CP de la cuenta. Si se elige un CP en anónimo y luego se hace login, la web lo traslada al carrito de la cuenta. En la prueba del 2026-10-19 la cuenta tenía `default-delivery` 15002 y el carrito 15001 (heredado de la selección anónima). `clientData.settings.delivery_type` tampoco coincide necesariamente con `default-delivery`.

Envío: 4,50 €, gratis desde 72 € (`site.shipping_costs` y `site.minimum_shipping_free`; también viene por CP).

## Catálogo

Búsqueda de productos, sin cookies ni token:

```
POST https://catalog.gadisline.com/api/v3/catalog/products/search?page_number=0&rows_per_page=20&order_field=relevance&sort_type=asc
site-id: <site>
store-id: <store>
Content-Type: application/json
```

- Con `GET` responde 405.
- Texto libre: `{"search_term":"leche semidesnatada","minimum_should_match":1}` (lo que manda el buscador de la web). Un campo desconocido como `search_text` se ignora y devuelve todo el catálogo.
- Con `{"product_ids":["<uuid>", …], "minimum_should_match":1}` devuelve esos productos concretos (así lo hace la web tras añadir al carrito).
- El buscador de la web lanza a la vez la búsqueda de categorías (`{"search_term", "minimum_should_match":1, "category_level":3}`) y de marcas (`{"search_term", "minimum_should_match":1}`) en las rutas de abajo.

Cada producto trae, entre otros: `id` (UUID, el que usa el carrito), `product_code`, `commercial_description[{language,value}]`, `price`, `price_kilo_litre`, `weight` (`U` = unidad), `min_weight`/`max_weight`, `category`, `image`, `slug`, `offers`, `brand_description`.

- Por categoría: `{"category_ids":["<uuid>"], "minimum_should_match":1}` (probado: «Aceite oliva» → 21 productos). Se puede combinar con `search_term`. El JS también manda `brands` como filtro opcional.

Árbol de categorías (sin cookies ni token):

```
GET https://catalog.gadisline.com/api/v3/catalog/categories?language=ES
site-id: <site>
store-id: <store>
```

→ `{"categories":[{"id","name","image","slug","nested_categories":{"categories":[…],"has_more_elements"}}]}`, tres niveles. El `slug` es la ruta completa, p. ej. `/alimentacion/aceites-y-vinagres/aceite-oliva`, la misma que la URL de la web.

Otras rutas vistas al cargar la portada (sin probar a fondo):

- `POST https://catalog.gadisline.com/api/v3/catalog/categories/search?keep_request=true&order_field=relevance&sort_type=desc`
- `POST https://catalog.gadisline.com/api/v3/catalog/brands/search?keep_request=true&order_field=relevance`

## Sesión de la web (cookies)

`GET https://www.gadisline.com/` deja estas cookies en `www.gadisline.com`:

| Cookie | Para qué | Duración |
|---|---|---|
| `next-site` | Sesión cifrada de la web (tienda, CP, carrito). | 14 días |
| `__Host-next-auth.csrf-token` | CSRF de NextAuth. | Sesión |
| `__Secure-next-auth.callback-url` | NextAuth. | Sesión |
| `__Secure-next-auth.session-token.0` / `.1` | Solo con login. Sesión de NextAuth partida en dos cookies (~5 KB). | 30 días |

Un cliente HTTP con un *cookie jar* basta para todo lo de abajo. No hace falta navegador.

## Carrito

### Crear carrito (anónimo)

```
POST https://www.gadisline.com/api/config/cart
Content-Type: application/json

{"store_id":"<store>","postal_code":"15001","delivery_type":"HOME_DELIVERY","comments":""}
```

Devuelve el carrito (`id`, `products`, `total_cart_price`, …). `delivery_type` es `HOME_DELIVERY` o `STORE_PICKUP`. Es lo que hace la web al elegir código postal.

Con sesión no hace falta crearlo: la cuenta ya tiene uno. Su id viene en el HTML de `GET https://www.gadisline.com/` (con las cookies), en `__NEXT_DATA__` → `props.pageProps.cart.id`, junto con `store_id` y `postal_code` del perfil.

### Añadir, cambiar cantidad o quitar

```
PUT https://www.gadisline.com/api/config/updateProduct
Content-Type: application/json

{"store_id":"<store>","cartId":"<cart>","productId":"<uuid producto>","amount":1,
 "preparation_mode_id":null,"product_note":null,"substitution_type":null,"summaryCheckout":false}
```

- `amount` es la cantidad final, no un incremento. `amount: 0` quita el producto.
- Productos al peso (`weight: "P"` en el catálogo): `amount` va en **kilos**. Comprobado el 2026-10-20 capturando la web con el plátano de Canarias (`min_weight` y `scale` 0,165 kg, "peso aproximado de la unidad"): la ficha muestra la cantidad en kilos, el botón «+» sube de `scale` en `scale` (0,165 → 0,330) sin llamar al servidor, y «Añadir» manda `amount: 0.33` con `preparation_mode_id: null`; el carrito devuelve `amount: 0.33` y `line_price: 0.91` (0,33 × 2,75 €/kg, redondeado).
  - La API **no valida** la cantidad: acepta valores que no son múltiplo de `scale` (0,2) e incluso por debajo de `min_weight` (0,1 con mínimo 0,165), y cobra en proporción. Solo la interfaz de la web limita a múltiplos de `scale`.
  - El CLI rechaza cantidades fuera de `min_weight`/`max_weight` y, por decisión, no exige múltiplos de `scale`.
  - `preparation_mode_id` (preparación en charcutería, pescadería…): ninguno de los 1.327 productos de Frescos de la tienda de prueba (574 al peso) trae `preparation_methods`, así que se manda siempre `null`.
  - Para volver a comprobarlo contra la web real, con una sesión preparada con `GADIS_HOME=<dir> gadis login`: `GADIS_TEST_HOME=<dir> go test -tags integration -run TestRealCartByWeight -v ./internal/cli/` (añade dos porciones mínimas, compara cantidad y `line_price` y deja el carrito como estaba).
- Devuelve el carrito completo: cada línea con `product_id`, `product_name`, `amount`, `line_price`, `thumbnail`…, y los totales `total_product_price`, `costs` (envío), `total_cart_price`.
- Funciona igual en anónimo y con sesión (con sesión, el carrito trae `client_id`).

### Leer el carrito

- Con sesión: `GET https://cart.gadisline.com/api/v3/carts/<cart>` con `Authorization: Bearer <accessToken>`, `site-id` y `store-id`.
- `GET https://www.gadisline.com/api/config/cart` **no responde** (se queda colgado). No usarlo.
- En anónimo no hay lectura sin token conocida: guardar el último carrito que devuelve `updateProduct`.

### Llamar a `cart.gadisline.com` sin token

`POST /carts` crea un carrito, pero `POST /carts/<id>/products` responde `401` con `www-authenticate: Bearer`. Para escribir sin token hay que usar `/api/config/*`.

Otras rutas `/api/config/*` vistas en el JS y sin probar: `addMultipleProducts`, `removeCartProducts`, `clearCart`, `refreshCart`, `persistantCart`, `checkPostalCode`, `updateSchedule`, `deleteSchedule`, `checkout`.

## Login

- NextAuth con proveedor **Keycloak**: servidor `https://auth.gadisline.com/realms/gadisline`, `client_id=gadisline`, PKCE (`S256`).
- `GET https://www.gadisline.com/api/auth/session` (con cookies) devuelve `{"expires", "token":{"accessToken"}}`. El `accessToken` es un JWT de Keycloak de **10 minutos** (`scope: openid email profile`).
- NextAuth **renueva el `accessToken` solo**, en el servidor: tras caducar el primero (emitido 21:19:19, caducaba 21:29:19), `GET /api/auth/session` devolvió uno nuevo (emitido 21:28:54) sin error. Al renovar, NextAuth reescribe las cookies `__Secure-next-auth.session-token.*`, así que el cliente debe **guardar el cookie jar tras cada petición**. Para llamar a los microservicios, pedir `/api/auth/session` justo antes y usar ese token.

### Flujo programático (sin navegador)

Sin CAPTCHA ni segundo factor en el formulario de Keycloak (comprobado el 2026-10-19):

1. `GET https://www.gadisline.com/api/auth/csrf` → `{"csrfToken"}` (y la cookie de CSRF).
2. `POST https://www.gadisline.com/api/auth/signin/keycloak` como formulario con `csrfToken`, `callbackUrl=https://www.gadisline.com/` y `json=true` → `{"url"}` con la URL de autorización de Keycloak (con `state` y `code_challenge`).
3. `GET` esa URL → HTML con `<form id="loginForm" action="…/login-actions/authenticate?session_code=…&execution=…&client_id=gadisline&tab_id=…">` y campos `username` y `password`.
4. `POST` al `action` del formulario con `username` y `password` (manteniendo las cookies de `auth.gadisline.com`) → redirección a `https://www.gadisline.com/api/auth/callback/keycloak?code=…`.
5. Seguir la redirección **con GET**: NextAuth deja `__Secure-next-auth.session-token.*` y redirige a la portada. Si el callback se sigue con POST (p. ej. `curl -X POST -L`), NextAuth responde `400 /api/auth/error` ("This action with HTTP POST is not supported by NextAuth.js").

**SMS:** el formulario de Keycloak no pidió código en la prueba. El SMS de 6 dígitos que ve el usuario en su primer login es, según el JS, la **validación del teléfono** de la web (`ValidatePhoneBySMS`; `clients/<id>/sms-phone-validation`), no un segundo factor de Keycloak. Se hace una vez y queda `clientData.phone_verified: true`. Aun así, Keycloak podría activar un OTP condicional; el CLI debe detectar cualquier formulario que no sea el esperado y no darlo por bueno.

Probado con credenciales reales el 2026-10-19: la sesión creada así sirve igual que la del navegador (carrito de la cuenta, `/api/auth/session` con token) y dura 30 días. Puede convivir con la sesión del navegador.

## Checkout (sin probar)

Visto en el JS de la web; nada de esto se ha ejecutado.

- Calendario de franjas, **público** (sin token):
  ```
  GET https://store.gadisline.com/api/v3/stores/<store>/calendar?delivery_type=HOME_DELIVERY&init_date=2026-10-20&end_date=2026-10-23&postal_code=15001
  site-id: <site>
  store-id: <store>
  ```
  Devuelve `[{"date","schedule_ranges":[{"id","init_time","end_time","cut_time","cut_day","available","active",…}]}]`.
- Reservar franja: `/api/config/updateSchedule` (o `cart.gadisline.com/api/v3/carts/<cart>/schedule`). Quitarla: `/api/config/deleteSchedule`.
- Crear el pedido: `/api/config/checkout` (o `POST cart.gadisline.com/api/v3/carts/<cart>/checkout`). Campos vistos: `delivery_type`, `shipping_address_id`, `delivery_date`, `schedule_range_id`, `payment_method_id`, `credit_card_id`, `payment_url_ok`, `substitution_type` (por defecto `DELETE`), `commercial_communications_accepted`…
- Pago con **tarjeta o Bizum**, con **preautorización** del importe (`/stores/payment-method-types/<id>/pre-authorization-amount`). El campo `payment_url_ok` sugiere una pasarela con redirección: probablemente el pago final necesita que el usuario abra un enlace.
- Pedidos: `order.gadisline.com` → `/orders/clients/<clientId>`, `/orders/<id>/clients/me`.
