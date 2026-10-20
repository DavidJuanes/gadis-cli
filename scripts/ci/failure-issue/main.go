// Command failure-issue keeps one GitHub issue per scheduled workflow in sync
// with the workflow's latest result: a failure opens a needs-triage issue (or
// comments on the one already open), and the next success closes it.
//
// It is meant to run as the last job of a scheduled workflow:
//
//	go run ./scripts/ci/failure-issue --status "${{ needs.<job>.result }}"
//
// with GH_TOKEN set and the standard GITHUB_* variables of Actions.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func main() {
	if err := run(os.Args[1:], os.Getenv); err != nil {
		fmt.Fprintln(os.Stderr, "failure-issue:", err)
		os.Exit(1)
	}
}

func run(args []string, getenv func(string) string) error {
	fs := flag.NewFlagSet("failure-issue", flag.ContinueOnError)
	status := fs.String("status", "", "result of the watched job: success, failure, cancelled or skipped")
	if err := fs.Parse(args); err != nil {
		return err
	}

	workflow := getenv("GITHUB_WORKFLOW")
	if workflow == "" {
		return errors.New("GITHUB_WORKFLOW is not set")
	}
	runURL := fmt.Sprintf("%s/%s/actions/runs/%s",
		getenv("GITHUB_SERVER_URL"), getenv("GITHUB_REPOSITORY"), getenv("GITHUB_RUN_ID"))
	marker := fmt.Sprintf("<!-- failure-issue: %s -->", workflow)

	ctx := context.Background()
	switch *status {
	case "failure":
		open, err := findOpenIssue(ctx, marker)
		if err != nil {
			return err
		}
		if open != 0 {
			return gh(ctx, fmt.Sprintf("Ha vuelto a fallar: %s\n", runURL),
				"issue", "comment", strconv.Itoa(open), "--body-file", "-")
		}
		return gh(ctx, failureBody(workflow, runURL, marker),
			"issue", "create",
			"--title", "Falla el workflow programado "+workflow,
			"--label", "needs-triage",
			"--body-file", "-")
	case "success":
		open, err := findOpenIssue(ctx, marker)
		if err != nil || open == 0 {
			return err
		}
		return gh(ctx, "", "issue", "close", strconv.Itoa(open),
			"--comment", fmt.Sprintf("Vuelve a pasar: %s\n", runURL))
	case "cancelled", "skipped":
		// Says nothing about the health of the workflow.
		return nil
	default:
		return fmt.Errorf("unknown --status %q (want success, failure, cancelled or skipped)", *status)
	}
}

func failureBody(workflow, runURL, marker string) string {
	return fmt.Sprintf("%s\nHa fallado el workflow programado `%s`.\n\nEjecución: %s\n\n"+
		"Este issue lo gestiona `scripts/ci/failure-issue`: los siguientes fallos se comentan aquí "+
		"y se cierra solo cuando el workflow vuelve a pasar.\n", marker, workflow, runURL)
}

// findOpenIssue returns the number of the open issue whose body carries
// marker, or 0 if there is none. It filters locally instead of using GitHub
// search, whose index lags behind and could duplicate the issue.
func findOpenIssue(ctx context.Context, marker string) (int, error) {
	out, err := ghOutput(ctx, "", "issue", "list", "--state", "open", "--limit", "1000", "--json", "number,body")
	if err != nil {
		return 0, err
	}
	var issues []struct {
		Number int    `json:"number"`
		Body   string `json:"body"`
	}
	if err := json.Unmarshal(out, &issues); err != nil {
		return 0, fmt.Errorf("parse gh issue list: %w", err)
	}
	for _, is := range issues {
		if strings.Contains(is.Body, marker) {
			return is.Number, nil
		}
	}
	return 0, nil
}

// gh runs the gh CLI with body on stdin.
func gh(ctx context.Context, body string, args ...string) error {
	_, err := ghOutput(ctx, body, args...)
	return err
}

func ghOutput(ctx context.Context, stdin string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gh", args...) //nolint:gosec // G204: fixed binary, no shell; args are built by this program
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("gh %s: %w: %s", strings.Join(args[:2], " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}
