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

import 'package:flutter_test/flutter_test.dart';
import 'package:passbubble/core/jobs/job_runner.dart';

void main() {
  test('an overwrite import replaces managed keys and keeps the rest', () {
    final merged = mergeImportedData(
      {
        'username': 'old-user',
        'password': 'old-pw',
        'notes': 'old notes', // managed, absent from the import → cleared
        'custom_fields': [
          {'label': 'Old', 'value': 'x', 'type': 'text'},
        ],
        'tags': ['work', 'client'],
        'sign_in_with': 'google',
        'iban': 'AT00 1234',
        'other_client_key': {'nested': true},
      },
      {
        'username': 'new-user',
        'password': 'new-pw',
        'custom_fields': [
          {'label': 'New', 'value': 'y', 'type': 'text'},
        ],
      },
    );

    expect(merged['username'], 'new-user');
    expect(merged['password'], 'new-pw');
    expect(merged.containsKey('notes'), isFalse);
    expect((merged['custom_fields'] as List).single['label'], 'New');
    expect(merged['tags'], ['work', 'client']);
    expect(merged['sign_in_with'], 'google');
    expect(merged['iban'], 'AT00 1234');
    expect(merged['other_client_key'], {'nested': true});
  });
}
