package cli_test

import (
	"strings"
	"testing"

	"github.com/DavidJuanes/gadis-cli/internal/cli/clitest"
	"github.com/DavidJuanes/gadis-cli/internal/gadistest"
)

// Postal code: which store the catalog uses (flag > saved postal code >
// account > default store) and the postal-code command. The fake's postal
// codes: 15001 → store-1, 15002 → store-2 (the account's default delivery),
// 15405 → store-3.

type storeJSON struct {
	ID         string `json:"store_id"`
	PostalCode string `json:"postal_code"`
	Source     string `json:"source"`
}

func TestCatalogStorePrecedence(t *testing.T) {
	tests := []struct {
		name     string
		loggedIn bool
		saved    string // postal code saved with `gadis postal-code`
		flag     string
		want     storeJSON
	}{
		{"default store without session or postal code", false, "", "", storeJSON{defaultStoreID, "", "default"}},
		{"account's default delivery", true, "", "", storeJSON{"store-2", "15002", "account"}},
		{"saved postal code over account", true, "15405", "", storeJSON{"store-3", "15405", "config"}},
		{"saved postal code without session", false, "15405", "", storeJSON{"store-3", "15405", "config"}},
		{"flag over saved postal code and account", true, "15405", "15001", storeJSON{"store-1", "15001", "flag"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := gadistest.New(t)
			gadis := clitest.New(t, fake)
			if tt.loggedIn {
				gadis.Login(t)
			}
			if tt.saved != "" {
				gadis.Run(t, "postal-code", tt.saved).Success(t)
			}
			args := []string{"search", "leche", "--json"}
			if tt.flag != "" {
				args = append(args, "--postal-code", tt.flag)
			}

			var out struct {
				Store storeJSON `json:"store"`
			}
			gadis.Run(t, args...).Success(t).JSON(t, &out)

			if out.Store != tt.want {
				t.Errorf("store = %+v, want %+v", out.Store, tt.want)
			}
			reqs := fake.Requests("POST", "/catalog/products/search")
			if len(reqs) != 1 || reqs[0].Header.Get("store-id") != tt.want.ID {
				t.Errorf("catalog got %d searches, want one with store-id %q", len(reqs), tt.want.ID)
			}
		})
	}
}

func TestPostalCodeSaveShowAndClear(t *testing.T) {
	fake := gadistest.New(t)
	gadis := clitest.New(t, fake)
	gadis.Login(t)

	var st storeJSON
	gadis.Run(t, "postal-code", "15405", "--json").Success(t).JSON(t, &st)
	if st != (storeJSON{"store-3", "15405", "config"}) {
		t.Errorf("after saving: %+v", st)
	}

	res := gadis.Run(t, "postal-code").Success(t)
	if !strings.Contains(res.Stdout, "15405") || !strings.Contains(res.Stdout, "guardado") {
		t.Errorf("stdout = %q, want the saved postal code and its origin", res.Stdout)
	}

	res = gadis.Run(t, "postal-code", "--clear").Success(t)
	if !strings.Contains(res.Stdout, "15002") || !strings.Contains(res.Stdout, "cuenta") {
		t.Errorf("stdout = %q, want the account's postal code and its origin", res.Stdout)
	}
	gadis.Run(t, "postal-code", "--json").Success(t).JSON(t, &st)
	if st != (storeJSON{"store-2", "15002", "account"}) {
		t.Errorf("after clearing: %+v", st)
	}
}

func TestPostalCodeWithoutSessionOrSavedOne(t *testing.T) {
	gadis := clitest.New(t, gadistest.New(t))

	res := gadis.Run(t, "postal-code").Success(t)

	if !strings.Contains(res.Stdout, "tienda por defecto") {
		t.Errorf("stdout = %q, want the default store", res.Stdout)
	}
}

func TestPostalCodeWithoutDeliveryFails(t *testing.T) {
	fake := gadistest.New(t)
	gadis := clitest.New(t, fake)
	gadis.Run(t, "postal-code", "15405").Success(t)

	res := gadis.Run(t, "postal-code", "28001")

	if res.Code != 1 || !strings.Contains(res.Stderr, "28001") {
		t.Errorf("code %d, stderr %q; want a failure naming 28001", res.Code, res.Stderr)
	}
	var st storeJSON
	gadis.Run(t, "postal-code", "--json").Success(t).JSON(t, &st)
	if st.PostalCode != "15405" {
		t.Errorf("saved postal code = %+v, want 15405 untouched", st)
	}
}

func TestExpiredSessionFallsBackToDefaultStore(t *testing.T) {
	fake := gadistest.New(t)
	gadis := clitest.New(t, fake)
	gadis.Login(t)
	fake.ExpireSession()

	res := gadis.Run(t, "postal-code").Success(t)

	if !strings.Contains(res.Stderr, "caducado") || !strings.Contains(res.Stderr, "gadis login") {
		t.Errorf("stderr = %q, want a warning that the session expired", res.Stderr)
	}
	if !strings.Contains(res.Stdout, "tienda por defecto") {
		t.Errorf("stdout = %q, want the default store", res.Stdout)
	}
}

func TestAccountPostalCodeFailureFallsBackToDefaultStore(t *testing.T) {
	for _, path := range []string{"/api/auth/session", "/clients/client-1/default-delivery"} {
		t.Run(path, func(t *testing.T) {
			fake := gadistest.New(t)
			gadis := clitest.New(t, fake)
			gadis.Login(t)
			fake.Fail("GET", path, 500)

			res := gadis.Run(t, "search", "leche").Success(t)

			if !strings.Contains(res.Stderr, "aviso:") || !strings.Contains(res.Stderr, "tienda por defecto") {
				t.Errorf("stderr = %q, want a warning about the default store", res.Stderr)
			}
			reqs := fake.Requests("POST", "/catalog/products/search")
			if len(reqs) != 1 || reqs[0].Header.Get("store-id") != defaultStoreID {
				t.Errorf("catalog got %d searches, want one with the default store", len(reqs))
			}
		})
	}
}
