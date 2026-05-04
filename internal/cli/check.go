package cli

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jmreicha/cfgctl/internal/core"
	"github.com/spf13/cobra"
)

type checkRow struct {
	provider     string
	target       string
	status       core.CheckStatus
	latency      string
	note         string
	err          string
	unconfigured bool // provider enabled but returned no targets
}

func newCheckCmd() *cobra.Command {
	var timeout time.Duration

	cmd := &cobra.Command{
		Use:   "check [provider...]",
		Short: "Check live connectivity for providers",
		Long: `Verify that credentials and endpoints are reachable for registered providers.
If no providers are specified, all registered providers are checked.`,
		RunE: func(_ *cobra.Command, args []string) error {
			if err := validateProviders(args); err != nil {
				return err
			}

			ctx := context.Background()
			allResults := engine.Check(ctx, &core.CheckOptions{
				Providers: args,
				Timeout:   timeout,
			})

			rows, anyFail := buildCheckRows(allResults)
			if len(rows) == 0 {
				fmt.Println()
				fmt.Println(labelStyle.Render("No checks configured for the selected providers."))
				return nil
			}

			printCheckTable(rows)

			if anyFail {
				return errors.New("one or more checks failed")
			}
			return nil
		},
	}

	cmd.Flags().DurationVar(&timeout, "timeout", 10*time.Second, "per-check timeout")
	return cmd
}

func buildCheckRows(allResults []core.ProviderCheckResults) ([]checkRow, bool) {
	anyFail := false
	var rows []checkRow
	for _, pr := range allResults {
		if pr.Err != nil {
			rows = append(rows, checkRow{provider: pr.Provider, target: "—", latency: "—", err: pr.Err.Error()})
			anyFail = true
			continue
		}
		// nil means disabled/skipped — omit silently.
		// non-nil but empty means enabled with nothing configured — show a row.
		if pr.Results == nil {
			continue
		}
		if len(pr.Results) == 0 {
			rows = append(rows, checkRow{provider: pr.Provider, target: "—", latency: "—", note: "no targets configured", unconfigured: true})
			continue
		}
		for _, r := range pr.Results {
			if r.Status == core.CheckStatusFail {
				anyFail = true
			}
			latency := "—"
			if r.Latency > 0 {
				latency = r.Latency.Round(time.Millisecond).String()
			}
			rows = append(rows, checkRow{
				provider: pr.Provider,
				target:   r.Target,
				status:   r.Status,
				latency:  latency,
				note:     r.Note,
			})
		}
	}
	return rows, anyFail
}

func printCheckTable(rows []checkRow) {
	providerW, targetW, latencyW := len("PROVIDER"), len("TARGET"), len("LATENCY")
	for _, r := range rows {
		if len(r.provider) > providerW {
			providerW = len(r.provider)
		}
		if len(r.target) > targetW {
			targetW = len(r.target)
		}
		if len(r.latency) > latencyW {
			latencyW = len(r.latency)
		}
	}

	// fmt cannot see through ANSI escape codes so we pad manually.
	const statusWidth = 6 // len("STATUS")

	fmt.Println()
	fmt.Printf("  %-*s   %-*s   %-*s   %-*s   %s\n",
		providerW, "PROVIDER",
		targetW, "TARGET",
		statusWidth, "STATUS",
		latencyW, "LATENCY",
		"NOTE",
	)
	for _, r := range rows {
		note := r.note
		if r.err != "" {
			note = r.err
		}
		fmt.Printf("  %-*s   %-*s   %s   %-*s   %s\n",
			providerW, r.provider,
			targetW, r.target,
			formatStatusCol(r),
			latencyW, r.latency,
			note,
		)
	}
}

func formatStatusCol(r checkRow) string {
	const statusWidth = 6
	if r.unconfigured {
		// "—" is 1 visible char; pad to statusWidth with plain spaces.
		return "—" + fmt.Sprintf("%-*s", statusWidth-1, "")
	}
	label, styled := statusDisplay(r.status, r.err != "")
	// Pad the plain label to statusWidth, then replace the label portion with
	// the styled version — ANSI codes stay out of the padding calculation.
	padded := fmt.Sprintf("%-*s", statusWidth, label)
	return styled + padded[len(label):]
}

const statusError = "ERROR"

// statusDisplay returns the plain label and its styled version for a status.
// isErr overrides the status to ERROR when the provider itself returned an error.
func statusDisplay(s core.CheckStatus, isErr bool) (label, styled string) {
	if isErr {
		return statusError, errorStyle.Render(statusError)
	}
	switch s {
	case core.CheckStatusOK:
		return "OKAY", successStyle.Render("OKAY")
	case core.CheckStatusWarn:
		return "WARN", warningStyle.Render("WARN")
	case core.CheckStatusFail:
		return statusError, errorStyle.Render(statusError)
	default:
		return statusError, errorStyle.Render(statusError)
	}
}
