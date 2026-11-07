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
	"errors"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/Gerry3010/passbubble/cli/internal/vault"
)

// resolveOptions narrows which entries resolveEntry may pick.
type resolveOptions struct {
	entryType string // only entries of this type count; "" = any
	username  string // only entries with this username count; "" = any
}

// wrongTypeError means the argument names entries, but none of the wanted type.
type wrongTypeError struct {
	arg, entryType string
}

func (e *wrongTypeError) Error() string {
	return fmt.Sprintf("entry '%s' is not a %s entry", e.arg, strings.ToUpper(e.entryType))
}

// unlockForUsernames unlocks the vault to read usernames. A variable so tests do
// not wait for a master password.
var unlockForUsernames = ensureUnlocked

// resolveEntry finds the one entry a command argument names: an entry ID, or an
// exact name that exactly one entry has. It never guesses. Names are not unique,
// so several matches are an error listing them, and a name that merely resembles
// the argument is no match. It lists all entries rather than using the search,
// which matches substrings and stops at 50 hits.
func resolveEntry(arg string, opts resolveOptions) (vault.Entry, error) {
	entries, err := v.ListEntries()
	if err != nil {
		return vault.Entry{}, err
	}

	var matches []vault.Entry
	for _, e := range entries {
		if e.ID == arg {
			matches = []vault.Entry{e}
			opts.username = "" // an ID needs no further narrowing
			break
		}
		if e.Name == arg {
			matches = append(matches, e)
		}
	}
	if len(matches) == 0 {
		return vault.Entry{}, fmt.Errorf("no entry named exactly '%s' (try 'pwmgr search %s')", arg, arg)
	}

	if opts.entryType != "" {
		typed := matches[:0:0]
		for _, e := range matches {
			if e.Type == opts.entryType {
				typed = append(typed, e)
			}
		}
		if len(typed) == 0 {
			return vault.Entry{}, &wrongTypeError{arg: arg, entryType: opts.entryType}
		}
		matches = typed
	}

	if opts.username != "" {
		if matches, err = filterByUsername(matches, opts.username); err != nil {
			return vault.Entry{}, err
		}
		if len(matches) == 0 {
			return vault.Entry{}, fmt.Errorf("no entry named exactly '%s' has the username '%s'", arg, opts.username)
		}
	}

	if len(matches) > 1 {
		return vault.Entry{}, ambiguousEntryError(arg, rowsWithUsernames(matches))
	}
	return matches[0], nil
}

// filterByUsername decrypts the candidates: usernames live in the encrypted data.
func filterByUsername(entries []vault.Entry, username string) ([]vault.Entry, error) {
	if !v.IsUnlocked() {
		if err := unlockForUsernames(); err != nil {
			return nil, err
		}
	}
	var out []vault.Entry
	for _, e := range entries {
		full, err := v.GetEntry(e.ID)
		if err != nil {
			return nil, fmt.Errorf("read entry %s: %w", e.ID, err)
		}
		if full.Data != nil && full.Data.Username == username {
			out = append(out, e)
		}
	}
	return out, nil
}

// ambiguousRow is one candidate in the error for several matches.
type ambiguousRow struct {
	id, username, host string
}

// rowsWithUsernames prepares the candidates for the error. Several commands work
// without the master password, but the usernames need it. A vault that stays
// locked or an unreadable entry costs the username, not the list of IDs.
func rowsWithUsernames(entries []vault.Entry) []ambiguousRow {
	unlocked := v.IsUnlocked()
	if !unlocked {
		fmt.Fprintf(os.Stderr, "Several entries match. Unlock the vault to list their usernames.\n")
		unlocked = unlockForUsernames() == nil
	}
	rows := make([]ambiguousRow, len(entries))
	for i, e := range entries {
		rows[i] = ambiguousRow{id: e.ID, username: "(locked)", host: vault.URLHost(e.URL)}
		if !unlocked {
			continue
		}
		rows[i].username = "(unreadable)"
		if full, err := v.GetEntry(e.ID); err == nil && full.Data != nil {
			rows[i].username = full.Data.Username
		}
	}
	return rows
}

// ambiguousEntryError lists the candidates so the user can pick one by ID.
func ambiguousEntryError(arg string, rows []ambiguousRow) error {
	orDash := func(s string) string {
		if s == "" {
			return "-"
		}
		return s
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d entries are named '%s', so nothing was done. Use the ID instead of the name:\n", len(rows), arg)
	tw := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "  ID\tUSERNAME\tHOST")
	for _, r := range rows {
		_, _ = fmt.Fprintf(tw, "  %s\t%s\t%s\n", r.id, orDash(r.username), orDash(r.host))
	}
	_ = tw.Flush()
	return errors.New(strings.TrimRight(b.String(), "\n"))
}
