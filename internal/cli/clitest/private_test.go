package clitest

import (
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/DavidJuanes/gadis-cli/internal/gadistest"
)

// recorder is a testing.TB that records what a test reports instead of
// reporting it, so a test can check the messages of a failure.
type recorder struct {
	testing.TB
	msgs            []string
	failed, skipped bool
}

func (r *recorder) log(args ...any) { r.msgs = append(r.msgs, fmt.Sprintln(args...)) }
func (r *recorder) logf(format string, args ...any) {
	r.msgs = append(r.msgs, fmt.Sprintf(format, args...))
}

func (r *recorder) Log(args ...any)                   { r.log(args...) }
func (r *recorder) Logf(format string, args ...any)   { r.logf(format, args...) }
func (r *recorder) Error(args ...any)                 { r.failed = true; r.log(args...) }
func (r *recorder) Errorf(format string, args ...any) { r.failed = true; r.logf(format, args...) }
func (r *recorder) Fail()                             { r.failed = true }
func (r *recorder) FailNow()                          { r.failed = true; runtime.Goexit() }
func (r *recorder) Fatal(args ...any)                 { r.Error(args...); runtime.Goexit() }
func (r *recorder) Fatalf(format string, args ...any) { r.Errorf(format, args...); runtime.Goexit() }
func (r *recorder) SkipNow()                          { r.skipped = true; runtime.Goexit() }
func (r *recorder) Skip(args ...any)                  { r.log(args...); r.SkipNow() }
func (r *recorder) Skipf(format string, args ...any)  { r.logf(format, args...); r.SkipNow() }
func (r *recorder) Failed() bool                      { return r.failed }
func (r *recorder) Skipped() bool                     { return r.skipped }

// record runs f with a recorder and returns it once f ends (also with Fatal).
func record(t testing.TB, f func(t testing.TB)) *recorder {
	r := &recorder{TB: t}
	done := make(chan struct{})
	go func() {
		defer close(done)
		f(r)
	}()
	<-done
	return r
}

func (r *recorder) output() string { return strings.Join(r.msgs, "\n") }

// A private CLI (the maintainer's real account) never prints what a command
// printed, not even when the test fails: only what went wrong.
func TestPrivateCLIHidesCommandOutputOnFailure(t *testing.T) {
	for name, c := range map[string]struct {
		run      func(t testing.TB, gadis *CLI)
		personal []string
	}{
		"JSON that doesn't decode": {
			run:      func(t testing.TB, gadis *CLI) { gadis.Run(t, "whoami", "--json").JSON(t, &[]int{}) },
			personal: []string{"@"}, // the account's email
		},
		"failing command": {
			run:      func(t testing.TB, gadis *CLI) { gadis.Run(t, "cart", "add", "p-secret").Success(t) },
			personal: []string{"p-secret"},
		},
		"cart that doesn't decode": {
			run:      func(t testing.TB, gadis *CLI) { gadis.Run(t, "cart", "--json").JSON(t, &[]int{}) },
			personal: []string{"p-secret", "Producto privado"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			fake := gadistest.New(t)
			fake.Cart.Lines = []gadistest.CartLine{{ProductID: "p-secret", Name: "Producto privado", Amount: 2, LinePrice: 1.5}}
			gadis := New(t, fake)
			gadis.Login(t)

			public := record(t, func(t testing.TB) { c.run(t, gadis) })
			if !public.failed || !strings.Contains(public.output(), c.personal[0]) {
				t.Fatalf("without Private the failure should show the output; got %q", public.output())
			}

			gadis.Private = true
			r := record(t, func(t testing.TB) { c.run(t, gadis) })
			if !r.failed {
				t.Fatal("the run did not fail")
			}
			for _, p := range c.personal {
				if strings.Contains(r.output(), p) {
					t.Errorf("failure message %q shows %q", r.output(), p)
				}
			}
		})
	}
}
