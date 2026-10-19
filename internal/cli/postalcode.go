package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/DavidJuanes/gadis-cli/internal/config"
)

func (a *app) postalCodeCmd() *cobra.Command {
	var clear bool
	cmd := &cobra.Command{
		Use:   "postal-code [CP]",
		Short: "Muestra o fija el código postal para buscar productos",
		Long: `Sin argumentos, muestra el código postal que usan search, categories,
category y product, y de dónde sale.

Con un CP, lo guarda en ~/.gadis/config.toml y se usará por defecto en lugar
del de tu cuenta. --clear vuelve a usar el de la cuenta.

Cada código postal lo sirve una tienda, y el catálogo y los precios dependen
de ella. El carrito siempre usa la tienda del propio carrito.`,
		Example: `  gadis postal-code
  gadis postal-code 15405
  gadis postal-code --clear`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if clear || len(args) == 1 {
				cfg, err := config.Load(a.env.ConfigDir)
				if err != nil {
					return err
				}
				cfg.PostalCode = ""
				if len(args) == 1 {
					pc, err := a.client.LookupPostalCode(ctx, args[0])
					if err != nil {
						return err
					}
					cfg.PostalCode = pc.PostalCode
				}
				if err := config.Save(a.env.ConfigDir, cfg); err != nil {
					return err
				}
			}
			st, err := a.resolveStore(ctx, "")
			if err != nil {
				return err
			}
			return a.emit(st, func(w io.Writer) {
				switch st.Source {
				case "config":
					fmt.Fprintf(w, "CP %s (guardado con `gadis postal-code`)\n", st.PostalCode)
				case "account":
					fmt.Fprintf(w, "CP %s (el de reparto por defecto de tu cuenta)\n", st.PostalCode)
				default:
					fmt.Fprintln(w, "Sin código postal: se usa la tienda por defecto de la web.")
					fmt.Fprintln(w, "Inicia sesión con `gadis login` o fija uno con `gadis postal-code <CP>`.")
				}
			})
		},
	}
	cmd.Flags().BoolVar(&clear, "clear", false, "olvidar el CP guardado y usar el de la cuenta")
	return cmd
}
