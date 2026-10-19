// Package cli implements the gadis command line.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/DavidJuanes/gadis-cli/internal/config"
	"github.com/DavidJuanes/gadis-cli/internal/gadis"
)

// Version is set at build time with -ldflags "-X …/internal/cli.Version=v1.2.3".
var Version = "dev"

type app struct {
	json   bool
	out    io.Writer
	errOut io.Writer
	client *gadis.Client
}

// Execute runs the CLI and returns the process exit code.
func Execute() int {
	a := &app{out: os.Stdout, errOut: os.Stderr}
	root := a.rootCmd()
	if err := root.ExecuteContext(context.Background()); err != nil {
		if a.json {
			_ = json.NewEncoder(os.Stderr).Encode(map[string]string{"error": err.Error()})
		} else {
			fmt.Fprintln(os.Stderr, "error:", err)
		}
		return 1
	}
	return 0
}

func (a *app) rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "gadis",
		Short: "Haz la compra en Gadisline (supermercados Gadis) desde la terminal",
		Long: `gadis busca productos en gadisline.com y gestiona el carrito de tu cuenta.

La búsqueda y las categorías funcionan sin cuenta. El carrito necesita
iniciar sesión con ` + "`gadis login`" + `. La sesión (cookies, nunca la contraseña)
se guarda en ~/.gadis/session.json y dura unos 30 días.`,
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			return a.init()
		},
	}
	root.PersistentFlags().BoolVar(&a.json, "json", false, "salida en JSON (para scripts y LLMs)")
	root.AddCommand(
		a.loginCmd(), a.logoutCmd(), a.whoamiCmd(),
		a.searchCmd(), a.categoriesCmd(), a.categoryCmd(), a.productCmd(),
		a.postalCodeCmd(), a.cartCmd(),
	)
	return root
}

func (a *app) init() error {
	if a.client != nil {
		return nil
	}
	path, err := config.SessionPath()
	if err != nil {
		return err
	}
	jar, err := gadis.LoadJar(path)
	if err != nil {
		return fmt.Errorf("leyendo la sesión %s: %w", path, err)
	}
	a.client = gadis.New(jar)
	return nil
}

// emit prints v as JSON with --json, or calls text otherwise.
func (a *app) emit(v any, text func(w io.Writer)) error {
	if a.json {
		enc := json.NewEncoder(a.out)
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	}
	text(a.out)
	return nil
}

// info prints progress messages to stderr, never polluting --json output.
func (a *app) info(format string, args ...any) {
	fmt.Fprintf(a.errOut, format+"\n", args...)
}

// store is the catalog a command works with and why.
type store struct {
	ID         string `json:"store_id"`
	PostalCode string `json:"postal_code,omitempty"`
	Source     string `json:"source"` // flag, config, account, default
}

// resolveStore picks the store for catalog commands: --postal-code, then the
// postal code saved with `gadis postal-code`, then the account's default
// delivery, then the website's default assortment.
func (a *app) resolveStore(ctx context.Context, flagCP string) (store, error) {
	cfg, err := config.Load()
	if err != nil {
		return store{}, err
	}
	for _, c := range []struct{ cp, source string }{{flagCP, "flag"}, {cfg.PostalCode, "config"}} {
		if c.cp == "" {
			continue
		}
		pc, err := a.client.LookupPostalCode(ctx, c.cp)
		if err != nil {
			return store{}, err
		}
		return store{ID: pc.StoreID, PostalCode: pc.PostalCode, Source: c.source}, nil
	}
	if a.client.LoggedIn() {
		d, err := a.accountDelivery(ctx)
		switch {
		case err == nil:
			return store{ID: d.StoreID, PostalCode: d.PostalCode, Source: "account"}, nil
		case errors.Is(err, gadis.ErrNotLoggedIn):
			a.info("aviso: la sesión ha caducado; uso la tienda por defecto (ejecuta `gadis login`)")
		default:
			a.info("aviso: no se pudo leer el código postal de la cuenta (%v); uso la tienda por defecto", err)
		}
	}
	return store{ID: gadis.DefaultStoreID, Source: "default"}, nil
}

func (a *app) accountDelivery(ctx context.Context) (gadis.Delivery, error) {
	acc, err := a.client.Account(ctx)
	if err != nil {
		return gadis.Delivery{}, err
	}
	return a.client.DefaultDelivery(ctx, acc.ClientID)
}

// euros formats a price the Spanish way: 1234.5 → "1.234,50 €".
func euros(v float64) string {
	s := strconv.FormatFloat(v, 'f', 2, 64)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	intPart, dec, _ := strings.Cut(s, ".")
	var b strings.Builder
	for i, r := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(r)
	}
	out := b.String() + "," + dec + " €"
	if neg {
		out = "-" + out
	}
	return out
}

// amount formats a cart amount: whole units without decimals, kilos with them.
func amount(v float64) string {
	if v == float64(int64(v)) {
		return strconv.FormatInt(int64(v), 10)
	}
	return strings.Replace(strconv.FormatFloat(v, 'f', -1, 64), ".", ",", 1)
}

// parseAmount accepts "2", "1.5" and "1,5".
func parseAmount(s string) (float64, error) {
	v, err := strconv.ParseFloat(strings.Replace(strings.TrimSpace(s), ",", ".", 1), 64)
	if err != nil || v < 0 {
		return 0, fmt.Errorf("cantidad no válida: %q", s)
	}
	return v, nil
}
