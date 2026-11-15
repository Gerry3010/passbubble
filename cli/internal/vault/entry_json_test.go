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

package vault

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// canon decodes JSON into generic values with numbers kept as literals, so
// two documents compare equal exactly when they hold the same keys and values.
func canon(t *testing.T, s string) any {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("decode %s: %v", s, err)
	}
	return v
}

func roundTrip(t *testing.T, in string) (EntryData, string) {
	t.Helper()
	var d EntryData
	if err := json.Unmarshal([]byte(in), &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	out, err := json.Marshal(&d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return d, string(out)
}

func TestEntryDataRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string // "" = same as in
	}{
		{
			name: "known fields only",
			in:   `{"username":"u","password":"p","notes":"n","period":30,"digits":6,"sign_in_with":"google"}`,
		},
		{
			name: "tags",
			in:   `{"username":"u","tags":["work","Finance","two words"]}`,
		},
		{
			name: "custom field types written by the app",
			in: `{"custom_fields":[
				{"label":"PIN","value":"1234","type":"password"},
				{"label":"Deploy key","value":"-----BEGIN-----","type":"ssh"},
				{"label":"2FA","value":"JBSWY3DPEHPK3PXP","type":"totp"},
				{"label":"Contract","value":"aGVsbG8=","type":"file","filename":"contract.pdf","mime_type":"application/pdf"},
				{"label":"Plain","value":"x","type":"text"}]}`,
		},
		{
			name: "unknown top-level keys of every JSON type",
			in: `{"username":"u",
				"future_string":"x",
				"future_big_number":12345678901234567890123,
				"future_float":1.5e-7,
				"future_bool":false,
				"future_null":null,
				"future_object":{"a":[1,2,{"b":"c"}],"empty":{}},
				"future_array":["x",1,null,true],
				"html":"<a href=\"x\">&</a>"}`,
		},
		{
			name: "unknown keys inside custom fields",
			in: `{"custom_fields":[
				{"label":"a","value":"b","type":"text","hidden":true,"meta":{"order":3,"by":["app"]}},
				{"label":"c","value":"d","note":null}]}`,
		},
		{
			name: "everything together",
			in: `{"password":"p","tags":["x"],"future":{"v":2},
				"custom_fields":[{"label":"f","value":"aGk=","type":"file","filename":"hi.txt","mime_type":"text/plain","sha256":"abc"}]}`,
		},
		{
			name: "server fields",
			in:   `{"username":"root","password":"pw","host":"web01.example.com","port":"2222","private_key":"-----BEGIN OPENSSH PRIVATE KEY-----\nb3Bl\n-----END OPENSSH PRIVATE KEY-----\n","key_passphrase":"secret"}`,
		},
		{
			name: "passkey with only known keys",
			in:   `{"username":"alice@example.com","password":"","passkeys":[` + docsPasskey + `]}`,
			want: `{"username":"alice@example.com","passkeys":[` + docsPasskey + `]}`,
		},
		{
			name: "passkey without the optional keys, not discoverable",
			in: `{"passkeys":[{"credential_id":"AQID","rp_id":"b.example","user_handle":"BAUG","user_name":"bob",
				"key_algorithm":-7,"private_key":"MIGH","sign_count":0,"discoverable":false,"created_at":"2026-01-01T00:00:00Z"}]}`,
		},
		{
			name: "unknown keys at entry level and inside passkeys",
			in: `{"username":"u","future":{"v":2},"passkeys":[
				{"credential_id":"AQID","rp_id":"b.example","user_handle":"BAUG","user_name":"bob",
				 "key_algorithm":-7,"private_key":"MIGH","sign_count":0,"discoverable":true,"created_at":"2026-01-01T00:00:00Z",
				 "backup_state":true,"transports":["internal","hybrid"],"meta":{"by":"app"},"nothing":null}]}`,
		},
		{
			name: "key differing only in case stays a separate key",
			in:   `{"username":"lower","Username":"Upper","TAGS":["shout"]}`,
		},
		{
			name: "empty known values are omitted, unknown ones kept",
			in:   `{"username":"","period":0,"tags":[],"custom_fields":[],"future":""}`,
			want: `{"future":""}`,
		},
		{
			name: "empty object",
			in:   `{}`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, out := roundTrip(t, c.in)
			want := c.want
			if want == "" {
				want = c.in
			}
			if got, exp := canon(t, out), canon(t, want); !reflect.DeepEqual(got, exp) {
				t.Fatalf("round trip changed the data\n got: %s\nwant: %s", out, want)
			}
		})
	}
}

// docsPasskey is the example record of docs/passkeys.md.
const docsPasskey = `{
	"credential_id": "q2D0bQ2u3E7XoWlS1yZbVw",
	"rp_id": "example.com",
	"rp_name": "Example",
	"user_handle": "dXNlci0xMjM0",
	"user_name": "alice@example.com",
	"user_display_name": "Alice",
	"key_algorithm": -7,
	"private_key": "MIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQg",
	"sign_count": 0,
	"discoverable": true,
	"prf_secret": "3q2-7wAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	"created_at": "2026-11-14T03:00:00Z"
}`

func TestPasskeyDecodesEveryDocumentedKey(t *testing.T) {
	d, _ := roundTrip(t, `{"passkeys":[`+docsPasskey+`]}`)
	want := Passkey{
		CredentialID: "q2D0bQ2u3E7XoWlS1yZbVw", RPID: "example.com", RPName: "Example",
		UserHandle: "dXNlci0xMjM0", UserName: "alice@example.com", UserDisplayName: "Alice",
		KeyAlgorithm: -7, PrivateKey: "MIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQg",
		SignCount: 0, Discoverable: true, PRFSecret: "3q2-7wAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		CreatedAt: "2026-11-14T03:00:00Z",
	}
	if len(d.Passkeys) != 1 || !reflect.DeepEqual(d.Passkeys[0], want) {
		t.Fatalf("passkeys = %+v", d.Passkeys)
	}
	if d.Passkeys[0].extra != nil {
		t.Errorf("a record of known keys has no extras: %v", d.Passkeys[0].extra)
	}
	if len(d.UnknownFields()) != 0 {
		t.Errorf("passkeys are modelled, not unknown: %+v", d.UnknownFields())
	}
}

// An edit of a passkey's known field leaves its unknown keys alone.
func TestPasskeyEditKeepsUnknownKeys(t *testing.T) {
	d, _ := roundTrip(t, `{"passkeys":[{"credential_id":"AQID","rp_id":"a.example","user_handle":"","user_name":"u",
		"key_algorithm":-7,"private_key":"k","sign_count":0,"discoverable":true,"created_at":"","future_pk":[1]}]}`)
	d.Passkeys[0].RPName = "A"
	d.Passkeys = append(d.Passkeys, Passkey{CredentialID: "BAUG", RPID: "b.example", KeyAlgorithm: -7})
	out, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"passkeys":[
		{"credential_id":"AQID","rp_id":"a.example","rp_name":"A","user_handle":"","user_name":"u",
		 "key_algorithm":-7,"private_key":"k","sign_count":0,"discoverable":true,"created_at":"","future_pk":[1]},
		{"credential_id":"BAUG","rp_id":"b.example","user_handle":"","user_name":"",
		 "key_algorithm":-7,"private_key":"","sign_count":0,"discoverable":false,"created_at":""}]}`
	if got, exp := canon(t, string(out)), canon(t, want); !reflect.DeepEqual(got, exp) {
		t.Fatalf("\n got: %s\nwant: %s", out, want)
	}
}

func TestEntryDataDecodesModelledFields(t *testing.T) {
	d, _ := roundTrip(t, `{"username":"lower","Username":"Upper","tags":["a","b"],
		"custom_fields":[{"label":"f","value":"v","type":"file","filename":"f.bin","mime_type":"application/octet-stream","x":1}]}`)

	if d.Username != "lower" {
		t.Errorf("Username = %q, want the exact-case key's value", d.Username)
	}
	if !slices.Equal(d.Tags, []string{"a", "b"}) {
		t.Errorf("Tags = %v", d.Tags)
	}
	cf := d.CustomFields[0]
	if cf.Type != "file" || cf.Filename != "f.bin" || cf.MimeType != "application/octet-stream" {
		t.Errorf("custom field = %+v", cf)
	}
	if _, ok := d.extra["Username"]; !ok {
		t.Error(`"Username" should be kept as an unknown key`)
	}
	if _, ok := cf.extra["x"]; !ok {
		t.Error(`custom field key "x" should be kept as an unknown key`)
	}
}

// An edit changes modelled fields and leaves unknown keys alone; clearing a
// modelled field really removes it.
func TestEntryDataEditKeepsUnknownKeys(t *testing.T) {
	d, _ := roundTrip(t, `{"password":"old","tags":["a"],"future":{"v":1},
		"custom_fields":[{"label":"k","value":"v","type":"password","color":"red"}]}`)

	d.Password = "new"
	d.Tags = nil
	d.CustomFields[0].Value = "v2"
	d.CustomFields = append(d.CustomFields, CustomField{Label: "added", Value: "x"})

	out, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"password":"new","future":{"v":1},"custom_fields":[
		{"label":"k","value":"v2","type":"password","color":"red"},
		{"label":"added","value":"x"}]}`
	if got, exp := canon(t, string(out)), canon(t, want); !reflect.DeepEqual(got, exp) {
		t.Fatalf("\n got: %s\nwant: %s", out, want)
	}
}

func TestEntryDataValueAndPointerEncodeAlike(t *testing.T) {
	d, _ := roundTrip(t, `{"username":"u","future":1}`)
	byValue, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	byPointer, err := json.Marshal(&d)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(byValue, byPointer) {
		t.Fatalf("value %s != pointer %s", byValue, byPointer)
	}
}

func TestEntryDataNull(t *testing.T) {
	var p *EntryData
	if err := json.Unmarshal([]byte(`null`), &p); err != nil || p != nil {
		t.Fatalf("null into *EntryData: p=%v err=%v", p, err)
	}
	d := EntryData{Username: "keep"}
	if err := json.Unmarshal([]byte(`null`), &d); err != nil || d.Username != "keep" {
		t.Fatalf("null into EntryData should be a no-op: %+v err=%v", d, err)
	}
}

func TestEntryDataInvalid(t *testing.T) {
	for _, in := range []string{`[]`, `"x"`, `{"period":"thirty"}`, `{"custom_fields":[1]}`, `{`,
		`{"passkeys":[1]}`, `{"passkeys":[{"key_algorithm":"ES256"}]}`, `{"port":22}`} {
		var d EntryData
		if err := json.Unmarshal([]byte(in), &d); err == nil {
			t.Errorf("%s: expected an error", in)
		}
	}
}

func TestJSONKeys(t *testing.T) {
	for _, k := range []string{"username", "sign_in_with", "custom_fields", "tags", "expires_at",
		"host", "port", "private_key", "key_passphrase", "passkeys"} {
		if !entryDataKeys[k] {
			t.Errorf("entryDataKeys misses %q", k)
		}
	}
	if entryDataKeys["extra"] || customFieldKeys["extra"] || passkeyKeys["extra"] {
		t.Error("the unexported extras map must not count as a key")
	}
	wantPasskey := map[string]bool{
		"credential_id": true, "rp_id": true, "rp_name": true, "user_handle": true, "user_name": true,
		"user_display_name": true, "key_algorithm": true, "private_key": true, "sign_count": true,
		"discoverable": true, "prf_secret": true, "created_at": true,
	}
	if !reflect.DeepEqual(passkeyKeys, wantPasskey) {
		t.Errorf("passkeyKeys = %v, want %v", passkeyKeys, wantPasskey)
	}
	want := map[string]bool{"label": true, "value": true, "type": true, "filename": true, "mime_type": true}
	if !reflect.DeepEqual(customFieldKeys, want) {
		t.Errorf("customFieldKeys = %v, want %v", customFieldKeys, want)
	}
}

func TestUnknownFieldsListWhatTheCLIDoesNotModel(t *testing.T) {
	var d EntryData
	const stored = `{
		"username": "octo",
		"zeta": "text",
		"alpha": {"nested": [1, 2]},
		"count": 3,
		"flag": false,
		"nothing": null,
		"empty": "",
		"custom_fields": [
			{"label": "PIN", "value": "1", "future_cf": "x"},
			{"label": "Plain", "value": "2"}
		]
	}`
	if err := json.Unmarshal([]byte(stored), &d); err != nil {
		t.Fatal(err)
	}
	want := []UnknownField{
		{"alpha", `{"nested": [1, 2]}`},
		{"count", "3"},
		{"flag", "false"},
		{"zeta", "text"},
		{"PIN.future_cf", "x"},
	}
	if got := d.UnknownFields(); !slices.Equal(got, want) {
		t.Fatalf("got  %+v\nwant %+v", got, want)
	}
}
