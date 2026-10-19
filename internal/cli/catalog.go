package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/DavidJuanes/gadis-cli/internal/gadis"
)

const postalCodeHelp = "código postal de reparto (por defecto: el guardado con `gadis postal-code` o el de la cuenta)"

func (a *app) searchCmd() *cobra.Command {
	var cp string
	var page, perPage int
	cmd := &cobra.Command{
		Use:   "search <texto>",
		Short: "Busca productos",
		Example: `  gadis search "leche semidesnatada"
  gadis search yogur --limit 5 --json`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := a.resolveStore(cmd.Context(), cp)
			if err != nil {
				return err
			}
			res, err := a.client.Search(cmd.Context(), st.ID, gadis.SearchQuery{
				Text: strings.Join(args, " "), Page: page - 1, PerPage: perPage,
			})
			if err != nil {
				return err
			}
			return a.emitProducts(st, res)
		},
	}
	addPaging(cmd, &page, &perPage)
	cmd.Flags().StringVar(&cp, "postal-code", "", postalCodeHelp)
	return cmd
}

func (a *app) categoriesCmd() *cobra.Command {
	var cp string
	var depth int
	cmd := &cobra.Command{
		Use:   "categories",
		Short: "Lista el árbol de categorías",
		Example: `  gadis categories
  gadis categories --depth 3 --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			st, err := a.resolveStore(cmd.Context(), cp)
			if err != nil {
				return err
			}
			tree, err := a.client.Categories(cmd.Context(), st.ID)
			if err != nil {
				return err
			}
			tree = prune(tree, depth)
			return a.emit(tree, func(w io.Writer) { printTree(w, tree, 0) })
		},
	}
	cmd.Flags().IntVar(&depth, "depth", 2, "niveles a mostrar (1-3)")
	cmd.Flags().StringVar(&cp, "postal-code", "", postalCodeHelp)
	return cmd
}

func (a *app) categoryCmd() *cobra.Command {
	var cp string
	var page, perPage int
	cmd := &cobra.Command{
		Use:   "category <slug|id>",
		Short: "Lista los productos de una categoría",
		Long: `Lista los productos de una categoría. Acepta el slug completo
(/alimentacion/aceites-y-vinagres/aceite-oliva), su último tramo
(aceite-oliva) o el id que muestra ` + "`gadis categories`" + `.`,
		Example: `  gadis category aceite-oliva
  gadis category /lacteos --limit 50`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			st, err := a.resolveStore(ctx, cp)
			if err != nil {
				return err
			}
			tree, err := a.client.Categories(ctx, st.ID)
			if err != nil {
				return err
			}
			cat, ok := gadis.FindCategory(tree, args[0])
			if !ok {
				return fmt.Errorf("no existe la categoría %q (mira `gadis categories --depth 3`)", args[0])
			}
			res, err := a.client.Search(ctx, st.ID, gadis.SearchQuery{
				CategoryIDs: []string{cat.ID}, Page: page - 1, PerPage: perPage,
			})
			if err != nil {
				return err
			}
			if !a.json {
				fmt.Fprintf(a.out, "%s (%s)\n", cat.Name, cat.Slug)
			}
			return a.emitProducts(st, res)
		},
	}
	addPaging(cmd, &page, &perPage)
	cmd.Flags().StringVar(&cp, "postal-code", "", postalCodeHelp)
	return cmd
}

func (a *app) productCmd() *cobra.Command {
	var cp string
	cmd := &cobra.Command{
		Use:   "product <id>...",
		Short: "Muestra el detalle de uno o varios productos",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := a.resolveStore(cmd.Context(), cp)
			if err != nil {
				return err
			}
			products, err := a.client.Products(cmd.Context(), st.ID, args)
			if err != nil {
				return err
			}
			if len(products) == 0 {
				return fmt.Errorf("producto no encontrado en la tienda de tu código postal")
			}
			return a.emit(map[string]any{"store": st, "products": views(products)}, func(w io.Writer) {
				for i, p := range products {
					if i > 0 {
						fmt.Fprintln(w)
					}
					printProduct(w, p)
				}
			})
		},
	}
	cmd.Flags().StringVar(&cp, "postal-code", "", postalCodeHelp)
	return cmd
}

func addPaging(cmd *cobra.Command, page, perPage *int) {
	cmd.Flags().IntVar(page, "page", 1, "página de resultados")
	cmd.Flags().IntVar(perPage, "limit", 20, "resultados por página")
}

func (a *app) emitProducts(st store, res *gadis.SearchResult) error {
	out := map[string]any{"store": st, "page": res.Page, "products": views(res.Products)}
	return a.emit(out, func(w io.Writer) {
		pages := max(res.Page.TotalPages, 1)
		fmt.Fprintf(w, "%d resultados (página %d de %d)\n", res.Page.TotalResults, res.Page.Number+1, pages)
		for _, p := range res.Products {
			fmt.Fprintf(w, "%s  %10s  %s%s\n", p.ID, euros(p.Price), p.Name(), unitNote(p))
		}
	})
}

// productView is the --json form of a product: flat and in Spanish.
type productView struct {
	ID        string   `json:"id"`
	Code      int64    `json:"code"`
	Name      string   `json:"name"`
	Brand     string   `json:"brand,omitempty"`
	Price     float64  `json:"price"`
	UnitPrice float64  `json:"unit_price,omitempty"`
	Unit      string   `json:"unit,omitempty"` // what unit_price is per: kilo, litro…
	ByWeight  bool     `json:"by_weight"`      // amount in kilos
	MinWeight float64  `json:"min_weight,omitempty"`
	Step      float64  `json:"weight_step,omitempty"`
	Category  string   `json:"category"`
	Offers    []string `json:"offers,omitempty"`
	Image     string   `json:"image,omitempty"`
	URL       string   `json:"url"`
}

func views(ps []gadis.Product) []productView {
	out := make([]productView, 0, len(ps))
	for _, p := range ps {
		v := productView{
			ID: p.ID, Code: p.Code, Name: p.Name(), Brand: p.Brand, Price: p.Price,
			UnitPrice: p.UnitPrice, Unit: p.Unit(), ByWeight: p.ByWeight(),
			Category: p.Category.Name.ES(), Image: p.Image.Image, URL: p.URL(),
		}
		if p.ByWeight() {
			v.MinWeight, v.Step = p.MinWeight, p.Scale
		}
		for _, o := range p.Offers {
			v.Offers = append(v.Offers, strings.TrimSpace(o.Icon.Name+" hasta "+o.EndDate))
		}
		out = append(out, v)
	}
	return out
}

func unitNote(p gadis.Product) string {
	switch {
	case p.ByWeight():
		return "  (por kilo)"
	case p.UnitPrice > 0 && p.UnitPrice != p.Price && p.Unit() != "":
		return fmt.Sprintf("  (%s/%s)", euros(p.UnitPrice), p.Unit())
	}
	return ""
}

func printProduct(w io.Writer, p gadis.Product) {
	fmt.Fprintf(w, "%s\n", p.Name())
	fmt.Fprintf(w, "  id:        %s\n", p.ID)
	fmt.Fprintf(w, "  código:    %d\n", p.Code)
	if p.Brand != "" {
		fmt.Fprintf(w, "  marca:     %s\n", p.Brand)
	}
	fmt.Fprintf(w, "  precio:    %s%s\n", euros(p.Price), unitNote(p))
	if p.ByWeight() {
		fmt.Fprintf(w, "  venta:     al peso; cantidad en kilos (mínimo %s, de %s en %s)\n", amount(p.MinWeight), amount(p.Scale), amount(p.Scale))
	}
	fmt.Fprintf(w, "  categoría: %s\n", p.Category.Name.ES())
	for _, o := range p.Offers {
		fmt.Fprintf(w, "  oferta:    %s (hasta %s)\n", o.Icon.Name, o.EndDate)
	}
	if p.Image.Image != "" {
		fmt.Fprintf(w, "  imagen:    %s\n", p.Image.Image)
	}
	fmt.Fprintf(w, "  web:       %s\n", p.URL())
}

func prune(tree []gadis.Category, depth int) []gadis.Category {
	out := make([]gadis.Category, len(tree))
	for i, c := range tree {
		out[i] = c
		if depth <= 1 {
			out[i].Children = nil
		} else {
			out[i].Children = prune(c.Children, depth-1)
		}
	}
	return out
}

func printTree(w io.Writer, tree []gadis.Category, level int) {
	for _, c := range tree {
		fmt.Fprintf(w, "%s%s  %s\n", strings.Repeat("  ", level), c.Name, c.Slug)
		printTree(w, c.Children, level+1)
	}
}
