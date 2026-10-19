// Package browserlogin signs in to gadisline.com in a visible Chromium window
// where the user types their credentials, then copies the session cookies.
package browserlogin

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"

	"github.com/DavidJuanes/gadis-cli/internal/gadis"
)

const sessionCookie = "__Secure-next-auth.session-token"

// Options configures the browser login.
type Options struct {
	ProfileDir string        // Chromium profile, kept between logins
	Timeout    time.Duration // how long to wait for the user
	ExecPath   string        // Chromium/Chrome binary; empty = autodetect
}

// Login opens gadisline.com, waits until the user has logged in, copies the
// gadisline.com cookies into jar and closes the browser.
func Login(ctx context.Context, jar *gadis.Jar, opt Options) error {
	allocOpts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false),
		chromedp.UserDataDir(opt.ProfileDir),
		chromedp.WindowSize(1200, 900),
	)
	if opt.ExecPath != "" {
		allocOpts = append(allocOpts, chromedp.ExecPath(opt.ExecPath))
	}
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(ctx, allocOpts...)
	defer cancelAlloc()
	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx)
	defer cancelBrowser()

	if err := chromedp.Do(browserCtx, chromedp.Navigate(gadis.Production.Web+"/")); err != nil {
		return fmt.Errorf("no se pudo abrir Chromium: %w", err)
	}

	deadline := time.Now().Add(opt.Timeout)
	for {
		cookies, err := gadislineCookies(browserCtx)
		if err != nil {
			if browserCtx.Err() != nil {
				return errors.New("se cerró el navegador antes de completar el login")
			}
			return err
		}
		if hasSession(cookies) {
			jar.Clear()
			for _, c := range cookies {
				jar.Put(toStored(c))
			}
			if err := jar.Save(); err != nil {
				return err
			}
			_ = chromedp.Cancel(browserCtx)
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("no se completó el login en %s", opt.Timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

func gadislineCookies(ctx context.Context) ([]*network.Cookie, error) {
	res, err := chromedp.Call(ctx, network.GetCookies, network.GetCookiesParams{URLs: []string{
		gadis.Production.Web + "/",
		"https://auth.gadisline.com/",
	}})
	if err != nil {
		return nil, err
	}
	var out []*network.Cookie
	for _, c := range res.Cookies {
		if strings.HasSuffix(strings.TrimPrefix(c.Domain, "."), "gadisline.com") {
			out = append(out, c)
		}
	}
	return out, nil
}

func hasSession(cookies []*network.Cookie) bool {
	for _, c := range cookies {
		if c.Name == sessionCookie || c.Name == sessionCookie+".0" {
			return true
		}
	}
	return false
}

func toStored(c *network.Cookie) gadis.StoredCookie {
	s := gadis.StoredCookie{
		Name:     c.Name,
		Value:    c.Value,
		Domain:   strings.TrimPrefix(c.Domain, "."),
		HostOnly: !strings.HasPrefix(c.Domain, "."),
		Path:     c.Path,
		Secure:   c.Secure,
		HTTPOnly: c.HTTPOnly,
	}
	if !c.Session && c.Expires > 0 {
		sec, frac := math.Modf(c.Expires)
		s.Expires = time.Unix(int64(sec), int64(frac*1e9))
	}
	return s
}
