// Copyright (C) 2026 Gerald Hofbauer <info@geraldhofbauer.net>
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// runRoot runs pwmgr with a logged-out config and returns everything it printed.
func runRoot(t *testing.T, args ...string) (string, error) {
	t.Helper()
	resetSilenceUsage(rootCmd) // earlier runs in this process leave it set
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs(append([]string{"--config", writeLoggedOutConfig(t)}, args...))
	t.Cleanup(func() {
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		rootCmd.SetArgs(nil)
	})
	err := rootCmd.Execute()
	return out.String(), err
}

// resetSilenceUsage undoes what a run leaves on the shared command tree.
func resetSilenceUsage(c *cobra.Command) {
	c.SilenceUsage = false
	for _, sub := range c.Commands() {
		resetSilenceUsage(sub)
	}
}

func TestRuntimeErrorShowsNoUsage(t *testing.T) {
	// Logged out, so the command fails while running, without any network.
	out, err := runRoot(t, "tags")
	if err == nil || !strings.Contains(err.Error(), "not logged in") {
		t.Fatalf("want a not-logged-in error, got %v\n%s", err, out)
	}
	if strings.Contains(out, "Usage:") {
		t.Errorf("a runtime error must not print the usage:\n%s", out)
	}
	if !strings.Contains(out, "not logged in") {
		t.Errorf("the error itself should be printed:\n%s", out)
	}
}

func TestWrongInvocationShowsUsage(t *testing.T) {
	for name, args := range map[string][]string{
		"unknown flag":    {"tags", "--no-such-flag"},
		"too many args":   {"tags", "surplus"},
		"missing args":    {"import"},
		"invalid value":   {"generate", "--length", "many"},
		"unknown on root": {"--no-such-flag"},
	} {
		t.Run(name, func(t *testing.T) {
			out, err := runRoot(t, args...)
			if err == nil {
				t.Fatalf("want an error\n%s", out)
			}
			if !strings.Contains(out, "Usage:") {
				t.Errorf("a wrong invocation should print the usage:\n%s", out)
			}
		})
	}
}

// A subcommand with its own hook would replace the root's: cobra runs only the
// nearest PersistentPreRun, and usage would come back for its runtime errors.
func TestNoSubcommandReplacesTheRootHook(t *testing.T) {
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		for _, sub := range c.Commands() {
			if sub.PersistentPreRun != nil || sub.PersistentPreRunE != nil {
				t.Errorf("%s defines its own PersistentPreRun", sub.CommandPath())
			}
			walk(sub)
		}
	}
	walk(rootCmd)
}
