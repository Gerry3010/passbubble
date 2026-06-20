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

package importers

import (
	"reflect"
	"testing"
)

func TestParsePsono_URLFilterToMatchPatterns(t *testing.T) {
	data := []byte(`{
		"items": [
			{
				"type": "website_password",
				"name": "A Trust",
				"website_password_title": "A Trust",
				"website_password_url": "http://www.a-trust.at",
				"website_password_username": "user",
				"website_password_password": "secret",
				"website_password_url_filter": "www.a-trust.at"
			},
			{
				"type": "website_password",
				"name": "Multi",
				"website_password_title": "Multi",
				"website_password_url": "https://example.com",
				"website_password_password": "pw",
				"urlfilter": "example.com, *.example.com"
			}
		]
	}`)

	res, err := ParsePsono(data)
	if err != nil {
		t.Fatalf("ParsePsono: %v", err)
	}
	if len(res.Records) != 2 {
		t.Fatalf("want 2 records, got %d", len(res.Records))
	}

	if got, want := res.Records[0].MatchPatterns, []string{"www.a-trust.at"}; !reflect.DeepEqual(got, want) {
		t.Errorf("record 0 match patterns = %v, want %v", got, want)
	}
	if got, want := res.Records[1].MatchPatterns, []string{"example.com", "*.example.com"}; !reflect.DeepEqual(got, want) {
		t.Errorf("record 1 match patterns = %v, want %v", got, want)
	}
}
