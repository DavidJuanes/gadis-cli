// Package cli implements the gadis command line.
package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/DavidJuanes/gadis-cli/internal/config"
	"github.com/DavidJuanes/gadis-cli/internal/gadis"
)

// Version is set at build time with -ldflags "-X …/internal/cli.Version=v1.2.3".
var Version = "dev"

type app struct {
	env    Env
	json   bool
	out    io.Writer
	errOut io.Writer
	in     *bufio.Reader // shared so successive prompts don't lose buffered input
	client *gadis.Client
}

// Env is everything the CLI takes from the outside world. The binary uses the
// process's (see Execute); tests inject their own.
type Env struct {
	Args           []string // without the program name
	Stdin          io.Reader
	Stdout, Stderr io.Writer
	URLs           gadis.Endpoints   // Gadisline services
	Transport      http.RoundTripper // nil = http.DefaultTransport
	ConfigDir      string            // settings directory (GADIS_HOME)
	// ResolveConfigDir finds the settings directory when ConfigDir is empty.
	// It runs only when a command needs it, so --version and help work even
	// if it fails (e.g. without $HOME).
	ResolveConfigDir func() (string, error)
	IsTerminal       func() bool // whether stdin is an interactive terminal
	Getenv           func(string) string
}

// Execute runs the CLI with the process's arguments, standard streams,
// environment and the production services, and returns the exit code.
func Execute() int {
	return Run(context.Background(), Env{
		Args:             os.Args[1:],
		Stdin:            os.Stdin,
		Stdout:           os.Stdout,
		Stderr:           os.Stderr,
		URLs:             gadis.Production,
		ResolveConfigDir: config.Dir,
		IsTerminal:       func() bool { return term.IsTerminal(int(os.Stdin.Fd())) },
		Getenv:           os.Getenv,
	})
}

// Run runs the CLI in env and returns the exit code: 0 on success, 1 on error.
// With --json, errors go to stderr as {"error": "…"}.
func Run(ctx context.Context, env Env) int {
	if env.Stdin == nil {
		env.Stdin = strings.NewReader("")
	}
	if env.IsTerminal == nil {
		env.IsTerminal = func() bool { return false }
	}
	if env.Getenv == nil {
		env.Getenv = func(string) string { return "" }
	}
	a := &app{env: env, out: env.Stdout, errOut: env.Stderr, in: bufio.NewReader(env.Stdin)}
	root := a.rootCmd()
	root.SetArgs(env.Args)
	root.SetIn(env.Stdin)
	root.SetOut(env.Stdout)
	root.SetErr(env.Stderr)
	if err := root.ExecuteContext(ctx); err != nil {
		if a.json {
			_ = json.NewEncoder(env.Stderr).Encode(map[string]string{"error": err.Error()})
		} else {
			fmt.Fprintln(env.Stderr, "error:", err)
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
	if a.env.ConfigDir == "" {
		if a.env.ResolveConfigDir == nil {
			return errors.New("no se conoce el directorio de configuración")
		}
		dir, err := a.env.ResolveConfigDir()
		if err != nil {
			return fmt.Errorf("no se pudo determinar el directorio de configuración: %w", err)
		}
		a.env.ConfigDir = dir
	}
	path := config.SessionPath(a.env.ConfigDir)
	jar, err := gadis.LoadJar(path)
	if err != nil {
		return fmt.Errorf("leyendo la sesión %s: %w", path, err)
	}
	a.client = gadis.New(jar)
	a.client.URLs = a.env.URLs
	if a.env.Transport != nil {
		a.client.HTTP.Transport = a.env.Transport
	}
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
	cfg, err := config.Load(a.env.ConfigDir)
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
