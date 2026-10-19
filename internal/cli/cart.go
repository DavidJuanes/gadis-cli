package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/DavidJuanes/gadis-cli/internal/gadis"
)

func (a *app) cartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cart",
		Short: "Muestra y modifica el carrito de tu cuenta",
		Long: `Muestra y modifica el carrito de tu cuenta de Gadisline (requiere ` + "`gadis login`" + `).

Los productos se indican por su id (el UUID que muestran search, category y
product). Los productos al peso se piden en kilos (p. ej. 1,5).`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return a.showCart(cmd.Context()) },
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "get",
			Short: "Muestra el carrito",
			Args:  cobra.NoArgs,
			RunE:  func(cmd *cobra.Command, _ []string) error { return a.showCart(cmd.Context()) },
		},
		a.cartAddCmd(),
		&cobra.Command{
			Use:     "set <id> <cantidad>",
			Short:   "Fija la cantidad de un producto (0 lo quita)",
			Example: "  gadis cart set cd09a2d7-ee2b-4819-9222-1c9369b215f2 3",
			Args:    cobra.ExactArgs(2),
			RunE: func(cmd *cobra.Command, args []string) error {
				qty, err := parseAmount(args[1])
				if err != nil {
					return err
				}
				return a.changeCart(cmd.Context(), args[0], func(float64) float64 { return qty })
			},
		},
		&cobra.Command{
			Use:     "remove <id>...",
			Aliases: []string{"rm"},
			Short:   "Quita productos del carrito",
			Args:    cobra.MinimumNArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				for _, id := range args {
					if err := a.changeCart(cmd.Context(), id, func(float64) float64 { return 0 }); err != nil {
						return err
					}
				}
				return nil
			},
		},
		a.cartClearCmd(),
	)
	return cmd
}

func (a *app) cartAddCmd() *cobra.Command {
	var qty string
	cmd := &cobra.Command{
		Use:   "add <id>...",
		Short: "Añade productos al carrito (suma a lo que ya haya)",
		Example: `  gadis cart add cd09a2d7-ee2b-4819-9222-1c9369b215f2
  gadis cart add cd09a2d7-ee2b-4819-9222-1c9369b215f2 --qty 4`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			n, err := parseAmount(qty)
			if err != nil {
				return err
			}
			if n == 0 {
				return errors.New("--qty debe ser mayor que 0")
			}
			for _, id := range args {
				if err := a.changeCart(cmd.Context(), id, func(cur float64) float64 { return cur + n }); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&qty, "qty", "1", "cantidad a añadir (kilos si el producto es al peso)")
	return cmd
}

func (a *app) cartClearCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "clear",
		Short: "Vacía el carrito",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !yes {
				return errors.New("esto quita todos los productos del carrito: repite con --yes para confirmarlo")
			}
			ctx := cmd.Context()
			cart, err := a.client.Cart(ctx)
			if err != nil {
				return err
			}
			for _, l := range cart.Lines {
				if cart, err = a.client.SetAmount(ctx, cart, l.ProductID, 0); err != nil {
					return err
				}
			}
			return a.printCart(cart)
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "confirmar que se vacía el carrito")
	return cmd
}

func (a *app) showCart(ctx context.Context) error {
	cart, err := a.client.Cart(ctx)
	if err != nil {
		return err
	}
	return a.printCart(cart)
}

// changeCart computes a product's new amount from its current one and saves it.
func (a *app) changeCart(ctx context.Context, productID string, next func(current float64) float64) error {
	cart, err := a.client.Cart(ctx)
	if err != nil {
		return err
	}
	var current float64
	line, inCart := cart.Line(productID)
	if inCart {
		current = line.Amount
	}
	qty := next(current)

	name := line.Name
	if qty > 0 {
		// Check the product exists in the cart's store, and its weight rules.
		products, err := a.client.Products(ctx, cart.StoreID, []string{productID})
		if err != nil {
			return err
		}
		if len(products) == 0 {
			return fmt.Errorf("el producto %s no existe en la tienda del carrito (CP %s)", productID, cart.PostalCode)
		}
		p := products[0]
		name = p.Name()
		if err := checkAmount(p, qty); err != nil {
			return err
		}
	} else if !inCart {
		return fmt.Errorf("el producto %s no está en el carrito", productID)
	}

	updated, err := a.client.SetAmount(ctx, cart, productID, qty)
	if err != nil {
		return err
	}
	if !a.json {
		if qty == 0 {
			a.info("quitado: %s", name)
		} else {
			a.info("%s × %s", name, amount(qty))
		}
	}
	return a.printCart(updated)
}

func checkAmount(p gadis.Product, qty float64) error {
	if !p.ByWeight() {
		if qty != float64(int64(qty)) {
			return fmt.Errorf("%s se vende por unidades: la cantidad debe ser entera", p.Name())
		}
		return nil
	}
	if p.MinWeight > 0 && qty < p.MinWeight {
		return fmt.Errorf("%s se vende al peso: mínimo %s kg", p.Name(), amount(p.MinWeight))
	}
	if p.MaxWeight > 0 && qty > p.MaxWeight {
		return fmt.Errorf("%s se vende al peso: máximo %s kg", p.Name(), amount(p.MaxWeight))
	}
	return nil
}

func (a *app) printCart(c *gadis.Cart) error {
	return a.emit(c, func(w io.Writer) {
		if len(c.Lines) == 0 {
			fmt.Fprintln(w, "El carrito está vacío.")
			return
		}
		for _, l := range c.Lines {
			fmt.Fprintf(w, "%s  %5s × %s  %10s\n", l.ProductID, amount(l.Amount), l.Name, euros(l.LinePrice))
		}
		fmt.Fprintf(w, "Productos: %s · Envío: %s · Total: %s\n", euros(c.ProductsPrice), euros(c.ShippingCost), euros(c.Total))
		if c.MissingForFree > 0 {
			fmt.Fprintf(w, "Faltan %s para el envío gratis.\n", euros(c.MissingForFree))
		}
	})
}
