---
name: gadis
description: >-
  Shop at Gadis supermarkets (gadisline.com) with the `gadis` CLI: price a shopping list,
  find products, and fill the account's cart. Use when the user wants to shop, price or
  build a cart at Gadis / Gadisline, e.g. "haz la compra en Gadis", "añade X al carrito
  de Gadis", "¿cuánto cuesta esta lista en Gadis?". Gadis only, not other supermarkets.
---

# Gadis shop

The `gadis` CLI talks to the same private HTTP API as gadisline.com. Reading the catalog is free; the cart is the user's real cart on their real account.

## The gate: confirm before touching the cart

Search, `product`, `categories` and `cart get` have no side effects: run them freely. Before the **first** `cart add`/`set`/`remove`/`clear` of a task, show the user the **plan** (each list item → the exact product picked: id, name, format, price, quantity, and the expected total) and wait for an explicit OK. Matching is fuzzy, so the plan is where the user catches a wrong brand or size.

After the OK, change only what the plan says. If something in the plan turns out unavailable or different, stop and ask instead of substituting.

The CLI has no checkout yet. When the cart is ready, tell the user to finish the order on gadisline.com (same account, same cart).

## Setup

1. `gadis --version`. If missing, install it as the README of github.com/DavidJuanes/gadis-cli says.
2. `gadis whoami`. It must print `ok — sesión válida`. If it says there is no session, ask the user to run `gadis login` themselves (it prompts for the password) or `gadis login --browser`. The user types credentials; never ask for the password in chat, and never pass it on the command line yourself.
3. `~/.gadis/session.json` holds the session cookies: never print or copy it.

## Working a list

Use `--json` on every command you parse. Run `gadis <command> --help` for flags.

1. For each item, `gadis search "<text>" --limit 10 --json`. Product ids are UUIDs; use the `id` field verbatim.
2. Pick the product that matches what the user asked for. When several formats, brands or sizes fit and the list does not settle it, ask, showing name, format and price of each option.
3. Show the plan and pass the gate.
4. Apply it: `gadis cart add <id> --qty N` adds to what is already there; `gadis cart set <id> N` sets the final quantity. Check `gadis cart get --json` against the plan at the end and report the total, shipping (`costs`) and what is left for free shipping (`missing_amount_free`).

## Gotchas

- **Sold by weight** (`by_weight: true`, e.g. fruit): the quantity is in **kilos**, at least `min_weight`, in steps of `weight_step`. "3 plátanos" is not `--qty 3`: work out the kilos and say so in the plan.
- **Store:** each postal code is served by one store, and catalog and prices depend on it. Catalog commands use the account's default delivery postal code (or one saved with `gadis postal-code`, or `--postal-code`). The cart always uses the cart's own store; adding a product that does not exist there fails with "no existe en la tienda del carrito".
- `gadis cart clear` needs `--yes`. Add it only when the user asked to empty the cart.
- Shipping is 4,50 € and free from 72 € of products (it can vary by postal code; trust the cart's numbers).
