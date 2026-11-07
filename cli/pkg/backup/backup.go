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

// Package backup writes and restores local JSON backups of decrypted entries. The
// `pwmgr backup` commands and the TUI both go through it. A backup holds the whole
// entry data, so it holds secrets in the clear: its file is only for its owner.
package backup

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Gerry3010/passbubble/cli/internal/vault"
)

// File is the content of a backup file.
type File struct {
	CreatedAt string  `json:"created_at"`
	Server    string  `json:"server"`
	Email     string  `json:"email"`
	Entries   []Entry `json:"entries"`
}

// Entry is one backed up entry. Data keeps everything the entry holds, also keys
// the CLI does not model.
type Entry struct {
	Name string           `json:"name"`
	Type string           `json:"type"`
	URL  string           `json:"url,omitempty"`
	Data *vault.EntryData `json:"data,omitempty"`
}

// Source is what a backup reads. *vault.Vault is one.
type Source interface {
	ListEntries() ([]vault.Entry, error)
	GetEntry(id string) (*vault.Entry, error)
}

// Target is what a restore writes to. *vault.Vault is one.
type Target interface {
	CreateEntry(name, entryType, url string, data *vault.EntryData, folderID *string, createdAt, updatedAt string, matchPatterns []string) (*vault.Entry, error)
}

// Options are the details of Create that are not entries.
type Options struct {
	Server, Email string                // recorded in the file
	Progress      func(done, total int) // told about each entry, may be nil
}

// Result says what Create wrote.
type Result struct {
	Path  string
	Saved int
	Total int
}

// IncompleteError means the backup was written, but without the Skipped entries.
type IncompleteError struct {
	Skipped []string
}

func (e *IncompleteError) Error() string {
	return "backup is incomplete, these could not be read and are NOT in it: " + strings.Join(e.Skipped, "; ")
}

// DefaultDir is where backups go unless told otherwise.
func DefaultDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find the backup directory: %w", err)
	}
	return filepath.Join(home, "Documents", "pwmgr-backups"), nil
}

// NewPath creates dir if needed and returns the path of a new backup in it. name
// is optional: it defaults to the time, and must stay a name, not a path. A backup
// of that name that exists already is never replaced by this.
func NewPath(dir, name string) (string, error) {
	if name == "" {
		name = time.Now().Format("20060102-150405")
	}
	name = strings.TrimPrefix(strings.TrimSuffix(name, ".json"), "backup-")
	if name == "" || strings.ContainsAny(name, `/\`) {
		return "", fmt.Errorf("invalid backup name %q: it must not be a path", name)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create backup dir: %w", err)
	}
	path := filepath.Join(dir, "backup-"+name+".json")
	if _, err := os.Lstat(path); err == nil {
		return "", fmt.Errorf("%s already exists", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	return path, nil
}

// Create writes a backup of every entry to path, readable by its owner only.
// Entries that cannot be read do not stop it: the file holds the rest, and the
// error is an *IncompleteError naming them.
func Create(src Source, path string, opts Options) (Result, error) {
	listed, err := src.ListEntries()
	if err != nil {
		return Result{}, err
	}

	entries := make([]Entry, 0, len(listed))
	var skipped []string
	for i, e := range listed {
		if opts.Progress != nil {
			opts.Progress(i+1, len(listed))
		}
		full, err := src.GetEntry(e.ID)
		if err == nil && full.Data == nil {
			err = errors.New("entry has no data")
		}
		if err != nil {
			skipped = append(skipped, fmt.Sprintf("%s (id %s): %v", e.Name, e.ID, err))
			continue
		}
		entries = append(entries, Entry{Name: full.Name, Type: full.Type, URL: full.URL, Data: full.Data})
	}

	data, err := json.MarshalIndent(File{
		CreatedAt: time.Now().Format(time.RFC3339),
		Server:    opts.Server,
		Email:     opts.Email,
		Entries:   entries,
	}, "", "  ")
	if err != nil {
		return Result{}, err
	}
	if err := writePrivate(path, data); err != nil {
		return Result{}, fmt.Errorf("write backup: %w", err)
	}

	result := Result{Path: path, Saved: len(entries), Total: len(listed)}
	if len(skipped) > 0 {
		return result, &IncompleteError{Skipped: skipped}
	}
	return result, nil
}

// writePrivate writes to a temporary file with mode 0600 and moves it into place.
// A write that fails leaves an older backup alone, and an existing file does not
// lend its wider permissions to the new content.
func writePrivate(path string, data []byte) (err error) {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".pwmgr-backup-*")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}
	}()
	if _, err = tmp.Write(data); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// Read loads a backup file.
func Read(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read backup file: %w", err)
	}
	var file File
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse backup file: %w", err)
	}
	return &file, nil
}

// RestoreResult says what Restore did.
type RestoreResult struct {
	Restored int
	Failed   []string // one line per entry that could not be created
}

// Restore creates a new entry for every entry in the file, twins included. It
// never looks at what the vault holds, so entries that are still there exist
// twice afterwards. Failures do not stop it and end up in the result.
func Restore(dst Target, file *File, progress func(done, total int)) RestoreResult {
	var result RestoreResult
	for i, e := range file.Entries {
		if progress != nil {
			progress(i+1, len(file.Entries))
		}
		data := e.Data
		if data == nil {
			data = &vault.EntryData{}
		}
		data.DropStrayTOTPSecret() // older backups still hold them
		if _, err := dst.CreateEntry(e.Name, e.Type, e.URL, data, nil, "", "", nil); err != nil {
			result.Failed = append(result.Failed, fmt.Sprintf("%s: %v", e.Name, err))
			continue
		}
		result.Restored++
	}
	return result
}

// Info describes a backup file on disk.
type Info struct {
	Name    string
	Path    string
	Size    int64
	ModTime time.Time
}

// List returns the backups in dir, newest first. A directory that does not exist
// holds none.
func List(dir string) ([]Info, error) {
	files, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read backup directory: %w", err)
	}
	var backups []Info
	for _, f := range files {
		name := f.Name()
		if f.IsDir() || !strings.HasPrefix(name, "backup-") || !strings.HasSuffix(name, ".json") {
			continue
		}
		info, err := f.Info()
		if err != nil {
			continue
		}
		backups = append(backups, Info{Name: name, Path: filepath.Join(dir, name), Size: info.Size(), ModTime: info.ModTime()})
	}
	sort.Slice(backups, func(i, j int) bool { return backups[i].ModTime.After(backups[j].ModTime) })
	return backups, nil
}
