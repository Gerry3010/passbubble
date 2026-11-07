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

package importers_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/tobischo/gokeepasslib/v3"
	"github.com/tobischo/gokeepasslib/v3/wrappers"

	"github.com/Gerry3010/passbubble/backend/pkg/exporters"
	"github.com/Gerry3010/passbubble/backend/pkg/importers"
)

const loginTypeSecret = "JBSWY3DPEHPK3PXP"

// loginTypeFormats builds a file with one login in each format and parses it.
var loginTypeFormats = map[string]func(t *testing.T, password string) *importers.ImportResult{
	"bitwarden": func(t *testing.T, password string) *importers.ImportResult {
		res, err := importers.ParseBitwarden(fmt.Appendf(nil,
			`{"items":[{"type":1,"name":"X","login":{"username":"u","password":%q,"totp":%q}}]}`, password, loginTypeSecret))
		if err != nil {
			t.Fatal(err)
		}
		return res
	},
	"lastpass": func(t *testing.T, password string) *importers.ImportResult {
		res, err := importers.ParseCSV(fmt.Appendf(nil,
			"url,username,password,totp,extra,name,grouping,fav\nhttps://x.example,u,%s,%s,,X,,0\n", password, loginTypeSecret),
			importers.CSVFormatLastPass)
		if err != nil {
			t.Fatal(err)
		}
		return res
	},
	"psono": func(t *testing.T, password string) *importers.ImportResult {
		res, err := importers.ParsePsono(fmt.Appendf(nil,
			`{"items":[{"type":"website_password","name":"X","website_password_title":"X",
				"website_password_username":"u","website_password_password":%q,"website_password_totp_code":%q}]}`,
			password, loginTypeSecret))
		if err != nil {
			t.Fatal(err)
		}
		return res
	},
	"1password": func(t *testing.T, password string) *importers.ImportResult {
		data, err := exporters.Export1PUX([]exporters.EntryRecord{
			{Type: "password", Name: "X", Username: "u", Password: password, TOTPSecret: loginTypeSecret},
		})
		if err != nil {
			t.Fatal(err)
		}
		res, err := importers.Parse1PUX(data)
		if err != nil {
			t.Fatal(err)
		}
		return res
	},
	"1password custom field": func(t *testing.T, password string) *importers.ImportResult {
		data, err := exporters.Export1PUX([]exporters.EntryRecord{{
			Type: "password", Name: "X", Username: "u", Password: password,
			CustomFields: []exporters.CustomField{{Label: "Code", Value: loginTypeSecret, Type: "totp"}},
		}})
		if err != nil {
			t.Fatal(err)
		}
		res, err := importers.Parse1PUX(data)
		if err != nil {
			t.Fatal(err)
		}
		return res
	},
	"keepass": func(t *testing.T, password string) *importers.ImportResult {
		entry := gokeepasslib.NewEntry()
		entry.Values = append(entry.Values,
			gokeepasslib.ValueData{Key: "Title", Value: gokeepasslib.V{Content: "X"}},
			gokeepasslib.ValueData{Key: "UserName", Value: gokeepasslib.V{Content: "u"}},
			gokeepasslib.ValueData{Key: "Password", Value: gokeepasslib.V{Content: password, Protected: wrappers.NewBoolWrapper(true)}},
			gokeepasslib.ValueData{Key: "otp", Value: gokeepasslib.V{Content: loginTypeSecret}},
		)
		group := gokeepasslib.NewGroup()
		group.Name = "Root"
		group.Entries = append(group.Entries, entry)
		db := gokeepasslib.NewDatabase()
		db.Credentials = gokeepasslib.NewPasswordCredentials("master")
		db.Content.Root = &gokeepasslib.RootData{Groups: []gokeepasslib.Group{group}}
		if err := db.LockProtectedEntries(); err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		if err := gokeepasslib.NewEncoder(&buf).Encode(db); err != nil {
			t.Fatal(err)
		}
		res, err := importers.ParseKeePass(bytes.NewReader(buf.Bytes()), "master")
		if err != nil {
			t.Fatal(err)
		}
		return res
	},
}

func TestLoginWithPasswordAndSecretStaysAPasswordEntry(t *testing.T) {
	for name, parse := range loginTypeFormats {
		t.Run(name, func(t *testing.T) {
			res := parse(t, "pw")
			if len(res.Records) != 1 {
				t.Fatalf("want 1 record, got %d", len(res.Records))
			}
			rec := res.Records[0]
			if rec.Type != "password" {
				t.Errorf("type = %q, want password", rec.Type)
			}
			if rec.Password != "pw" || rec.TOTPSecret != loginTypeSecret {
				t.Errorf("password %q, secret %q", rec.Password, rec.TOTPSecret)
			}
		})
	}
}

func TestLoginWithOnlyASecretBecomesATOTPEntry(t *testing.T) {
	for name, parse := range loginTypeFormats {
		t.Run(name, func(t *testing.T) {
			res := parse(t, "")
			if len(res.Records) != 1 {
				t.Fatalf("want 1 record, got %d", len(res.Records))
			}
			rec := res.Records[0]
			if rec.Type != "totp" {
				t.Errorf("type = %q, want totp", rec.Type)
			}
			if rec.TOTPSecret != loginTypeSecret {
				t.Errorf("secret = %q", rec.TOTPSecret)
			}
		})
	}
}

func TestATOTPFieldDoesNotRetypeACard(t *testing.T) {
	data, err := exporters.Export1PUX([]exporters.EntryRecord{{
		Type: "credit-card", Name: "Visa", CardNumber: "4111",
		CustomFields: []exporters.CustomField{{Label: "Code", Value: loginTypeSecret, Type: "totp"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	res, err := importers.Parse1PUX(data)
	if err != nil {
		t.Fatal(err)
	}
	rec := res.Records[0]
	if rec.Type != "credit-card" || rec.CardNumber != "4111" || rec.TOTPSecret != loginTypeSecret {
		t.Fatalf("record = %+v", rec)
	}
}
