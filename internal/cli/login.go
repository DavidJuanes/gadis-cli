package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/DavidJuanes/gadis-cli/internal/browserlogin"
	"github.com/DavidJuanes/gadis-cli/internal/config"
	"github.com/DavidJuanes/gadis-cli/internal/gadis"
)

func (a *app) loginCmd() *cobra.Command {
	var user, password string
	var browser bool
	var timeout time.Duration
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Inicia sesión en Gadisline",
		Long: `Inicia sesión en tu cuenta de Gadisline y guarda la sesión (solo cookies,
nunca la contraseña) en ~/.gadis/session.json. Dura unos 30 días.

Modo automático (por defecto), sin navegador. Las credenciales se toman, por
este orden, de:
  1. --user y --password
  2. las variables de entorno GADIS_EMAIL y GADIS_PASSWORD
  3. la terminal (la contraseña no se muestra)
Ojo: --password queda en el historial de la shell y es visible en ` + "`ps`" + `
mientras se ejecuta. Para automatizaciones es mejor GADIS_PASSWORD.

Si Gadisline pide un código de verificación (SMS), se pide por la terminal.
Sin terminal, el comando falla: usa entonces --browser.

Modo manual (--browser): abre Chromium para que inicies sesión tú, copia las
cookies y lo cierra.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			if browser {
				profile := config.BrowserProfile(a.env.ConfigDir)
				a.info("Abriendo Chromium: inicia sesión en Gadisline (Mi cuenta → Iniciar sesión). Esperando hasta %s…", timeout)
				err := browserlogin.Login(ctx, a.client.Jar, browserlogin.Options{ProfileDir: profile, Timeout: timeout})
				if err != nil {
					return err
				}
				return a.reportLogin(cmd)
			}

			user = firstNonEmpty(user, a.env.Getenv("GADIS_EMAIL"))
			password = firstNonEmpty(password, a.env.Getenv("GADIS_PASSWORD"))
			interactive := a.env.IsTerminal()
			if user == "" || password == "" {
				if !interactive {
					return errors.New("faltan credenciales: usa --user/--password, GADIS_EMAIL/GADIS_PASSWORD o --browser")
				}
				var err error
				if user == "" {
					if user, err = a.prompt("Email de Gadisline: "); err != nil {
						return err
					}
				}
				if password == "" {
					if password, err = a.readPassword("Contraseña: "); err != nil {
						return err
					}
				}
			}

			challenge, err := a.client.Login(ctx, user, password)
			password = ""
			for tries := 0; errors.Is(err, gadis.ErrCodeRequired); tries++ {
				if !interactive {
					return errors.New("Gadisline pide un código de verificación y no hay terminal para pedirlo: usa `gadis login --browser`")
				}
				if tries == 3 {
					return errors.New("demasiados intentos con el código de verificación")
				}
				if challenge.Message != "" {
					a.info("%s", challenge.Message)
				}
				code, perr := a.prompt("Código de verificación: ")
				if perr != nil {
					return perr
				}
				challenge, err = a.client.SubmitCode(ctx, challenge, code)
			}
			if err != nil {
				return err
			}
			return a.reportLogin(cmd)
		},
	}
	cmd.Flags().StringVar(&user, "user", "", "email de la cuenta")
	cmd.Flags().StringVar(&password, "password", "", "contraseña (mejor GADIS_PASSWORD: ver ayuda)")
	cmd.Flags().BoolVar(&browser, "browser", false, "iniciar sesión a mano en una ventana de Chromium")
	cmd.Flags().DurationVar(&timeout, "timeout", 5*time.Minute, "con --browser, cuánto esperar al login")
	return cmd
}

func (a *app) reportLogin(cmd *cobra.Command) error {
	acc, err := a.client.Account(cmd.Context())
	if err != nil {
		return fmt.Errorf("la sesión se creó pero no se pudo leer la cuenta: %w", err)
	}
	return a.emit(map[string]any{"ok": true, "email": acc.Email, "name": acc.Name}, func(w io.Writer) {
		fmt.Fprintf(w, "ok — sesión iniciada como %s\n", acc.Email)
	})
}

func (a *app) logoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Cierra la sesión y borra las cookies guardadas",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := a.client.Logout(cmd.Context()); err != nil {
				return err
			}
			return a.emit(map[string]any{"ok": true}, func(w io.Writer) { fmt.Fprintln(w, "ok — sesión cerrada") })
		},
	}
}

func (a *app) whoamiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Comprueba la sesión y muestra la cuenta",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			acc, err := a.client.Account(ctx)
			if err != nil {
				return err
			}
			d, err := a.client.DefaultDelivery(ctx, acc.ClientID)
			if err != nil {
				return err
			}
			out := map[string]any{
				"authenticated":    true,
				"email":            acc.Email,
				"name":             acc.Name,
				"default_delivery": d,
				"cart_id":          acc.Cart.ID,
				"cart_postal_code": acc.Cart.PostalCode,
			}
			return a.emit(out, func(w io.Writer) {
				fmt.Fprintf(w, "ok — sesión válida: %s (%s)\n", acc.Email, acc.Name)
				fmt.Fprintf(w, "Reparto por defecto: CP %s (%s)\n", d.PostalCode, deliveryName(d.DeliveryType))
				if acc.Cart.PostalCode != "" && acc.Cart.PostalCode != d.PostalCode {
					fmt.Fprintf(w, "El carrito está asociado al CP %s\n", acc.Cart.PostalCode)
				}
			})
		},
	}
}

func deliveryName(t string) string {
	switch t {
	case "HOME_DELIVERY":
		return "a domicilio"
	case "STORE_PICKUP":
		return "recogida en tienda"
	}
	return t
}

// prompt asks for a line on stderr and reads it from stdin.
func (a *app) prompt(label string) (string, error) {
	fmt.Fprint(a.errOut, label)
	line, err := a.in.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// readPassword asks for a password without echoing it when stdin is the
// process's terminal; otherwise (tests, pipes) it reads a plain line.
func (a *app) readPassword(label string) (string, error) {
	f, ok := a.env.Stdin.(*os.File)
	if !ok || !term.IsTerminal(int(f.Fd())) {
		return a.prompt(label)
	}
	fmt.Fprint(a.errOut, label)
	raw, err := term.ReadPassword(int(f.Fd()))
	fmt.Fprintln(a.errOut)
	return string(raw), err
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
