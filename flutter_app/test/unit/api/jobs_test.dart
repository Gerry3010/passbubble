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
import 'package:passbubble/core/api/models.dart';

void main() {
  test('CreateJobRequest serializes the import job', () {
    final json = const CreateJobRequest(
      type: 'import',
      format: 'bitwarden',
      dupStrategy: 'skip',
      totalItems: 42,
      clientName: 'Flutter',
    ).toJson();
    expect(json['type'], 'import');
    expect(json['format'], 'bitwarden');
    expect(json['dup_strategy'], 'skip');
    expect(json['total_items'], 42);
    expect(json['client_name'], 'Flutter');
  });

  test('UpdateJobRequest omits null fields and maps counts', () {
    final json = const UpdateJobRequest(
      status: 'completed',
      createdItems: 10,
      skippedItems: 2,
    ).toJson();
    expect(json['status'], 'completed');
    expect(json['created_items'], 10);
    expect(json['skipped_items'], 2);
    expect(json.containsKey('updated_items'), isFalse);
    expect(json.containsKey('error_message'), isFalse);
  });

  test('JobResponse parses ledger fields', () {
    final r = JobResponse.fromJson({
      'id': 'j1',
      'status': 'completed',
      'type': 'import',
      'format': 'bitwarden',
      'processed_items': 42,
      'total_items': 42,
      'created_items': 40,
      'updated_items': 0,
      'skipped_items': 2,
      'failed_items': 0,
      'created_at': '2026-06-19T00:00:00Z',
    });
    expect(r.id, 'j1');
    expect(r.createdItems, 40);
    expect(r.status, 'completed');
  });
}
