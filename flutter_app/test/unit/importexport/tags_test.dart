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

import 'dart:convert';
import 'dart:typed_data';

import 'package:archive/archive.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:passbubble/core/importexport/bitwarden_format.dart';
import 'package:passbubble/core/importexport/csv_format.dart';
import 'package:passbubble/core/importexport/entry_record.dart';
import 'package:passbubble/core/importexport/onepassword_format.dart';

const _tagged = EntryRecord(
  name: 'Mail',
  url: 'https://mail.example.com',
  username: 'alice',
  password: 'pw',
  notes: 'hello',
  tags: ['Work', 'client a'],
);
const _untagged = EntryRecord(name: 'Bank', username: 'bob', password: 'pw2');

/// The decoded `export.data` of a 1PUX archive.
Map<String, dynamic> _onePuxData(Uint8List bytes) {
  final file = ZipDecoder().decodeBytes(bytes).findFile('export.data')!;
  return jsonDecode(utf8.decode(file.content as List<int>)) as Map<String, dynamic>;
}

/// A 1PUX archive holding one login item with this [overview].
Uint8List _onePux(Map<String, dynamic> overview) {
  final json = utf8.encode(jsonEncode({
    'accounts': [
      {
        'vaults': [
          {
            'items': [
              {'categoryUuid': '001', 'overview': overview, 'details': <String, dynamic>{}},
            ],
          },
        ],
      },
    ],
  }));
  final archive = Archive()..addFile(ArchiveFile('export.data', json.length, json));
  return Uint8List.fromList(ZipEncoder().encode(archive)!);
}

void main() {
  group('own CSV', () {
    test('round trip through the tags column', () {
      final csv = exportCsv(const [_tagged, _untagged]);
      final lines = const LineSplitter().convert(csv);
      expect(lines.first, 'name,url,username,password,notes,tags');
      expect(lines[1], endsWith(',"Work, client a"'));
      expect(lines[2], endsWith(','), reason: 'an entry without tags has an empty cell');

      final parsed = parseCsv(csv, CsvImportFormat.generic);
      expect(parsed[0].tags, ['Work', 'client a']);
      expect(parsed[0].notes, 'hello');
      expect(parsed[0].password, 'pw');
      expect(parsed[1].tags, isEmpty);
    });

    test('splits like the tag input and normalises', () {
      final parsed = parseCsv(
        'name,url,username,password,notes,tags\n'
        'Mail,,alice,pw,,"#Work,  client   a , work,,${'x' * 41}"\n',
        CsvImportFormat.generic,
      );
      expect(parsed.single.tags, ['Work', 'client a']);
    });

    test('the column is found by name, not by position', () {
      final moved = parseCsv(
        'name,url,username,password,tags,notes\nMail,,alice,pw,"a, b",some notes\n',
        CsvImportFormat.generic,
      );
      expect(moved.single.tags, ['a', 'b']);

      final upper = parseCsv(
        'name,url,username,password,notes,TAGS\nMail,,alice,pw,,a\n',
        CsvImportFormat.generic,
      );
      expect(upper.single.tags, ['a']);

      // Something else after the known columns is not taken for tags.
      final foreign = parseCsv(
        'name,url,username,password,notes,folder\nMail,,alice,pw,,Work\n',
        CsvImportFormat.generic,
      );
      expect(foreign.single.tags, isEmpty);
    });

    test('a file without the column and a row without the cell have no tags', () {
      expect(
        parseCsv('name,url,username,password,notes\nMail,,alice,pw,n\n', CsvImportFormat.generic)
            .single
            .tags,
        isEmpty,
      );
      expect(
        parseCsv('name,url,username,password,notes,tags\nMail,,alice,pw\n', CsvImportFormat.generic)
            .single
            .tags,
        isEmpty,
      );
    });
  });

  group('1Password CSV', () {
    test('round trip through the Tags column', () {
      final csv = export1PasswordCsv(const [_tagged, _untagged]);
      expect(const LineSplitter().convert(csv).first,
          'Title,Username,Password,URL,Notes,Type,Tags');

      final parsed = parseCsv(csv, CsvImportFormat.onePassword);
      expect(parsed[0].tags, ['Work', 'client a']);
      expect(parsed[0].type, 'password', reason: 'the Type column stays where it was');
      expect(parsed[1].tags, isEmpty);
    });

    test('a file without the column has no tags', () {
      expect(
        parseCsv('Title,Username,Password,URL,Notes,Type\nMail,a,b,,,Login\n',
                CsvImportFormat.onePassword)
            .single
            .tags,
        isEmpty,
      );
    });
  });

  group('Bitwarden', () {
    test('round trip through the passbubble_tags field', () {
      const record = EntryRecord(
        name: 'Mail',
        username: 'alice',
        password: 'pw',
        tags: ['Work', 'client a'],
        customFields: [
          CustomFieldRecord(label: 'Recovery', value: 'ask the cat'),
          CustomFieldRecord(label: 'PIN', value: '4711', type: CustomFieldType.password),
        ],
      );
      final json = exportBitwarden(const [record, _untagged]);
      final items = (jsonDecode(json) as Map)['items'] as List;

      expect((items[0] as Map)['fields'], [
        {'name': 'Recovery', 'value': 'ask the cat', 'type': 0},
        {'name': 'PIN', 'value': '4711', 'type': 1},
        {'name': 'passbubble_tags', 'value': 'Work, client a', 'type': 0},
      ]);
      expect((items[1] as Map).containsKey('fields'), isFalse,
          reason: 'no tags, no field');

      final parsed = parseBitwarden(json);
      expect(parsed[0].tags, ['Work', 'client a']);
      expect(parsed[0].customFields.map((f) => f.label), ['Recovery', 'PIN'],
          reason: 'the tags field does not stay a custom field');
      expect(parsed[1].tags, isEmpty);
    });

    test('every item type takes its tags from the field', () {
      const fields = [
        {'name': 'passbubble_tags', 'value': '#a, b, A', 'type': 0},
        {'name': 'Kept', 'value': 'x', 'type': 0},
      ];
      final parsed = parseBitwarden(jsonEncode({
        'items': [
          {'type': 1, 'name': 'Login', 'login': <String, dynamic>{}, 'fields': fields},
          {'type': 2, 'name': 'Note', 'fields': fields},
          {'type': 3, 'name': 'Card', 'card': <String, dynamic>{}, 'fields': fields},
          {'type': 4, 'name': 'Identity', 'identity': <String, dynamic>{}, 'fields': fields},
        ],
      }));
      expect(parsed, hasLength(4));
      for (final rec in parsed) {
        expect(rec.tags, ['a', 'b'], reason: rec.name);
        expect(rec.customFields.single.label, 'Kept', reason: rec.name);
      }
    });

    test('a field with no value gives no tags and no custom field', () {
      final parsed = parseBitwarden(jsonEncode({
        'items': [
          {
            'type': 2,
            'name': 'Note',
            'fields': [
              {'name': 'passbubble_tags', 'value': null, 'type': 0},
            ],
          },
        ],
      }));
      expect(parsed.single.tags, isEmpty);
      expect(parsed.single.customFields, isEmpty);
    });
  });

  group('1PUX', () {
    test('round trip through overview.tags', () {
      final bytes = exportOnePux(const [_tagged, _untagged]);
      final items = ((_onePuxData(bytes)['accounts'] as List).single['vaults'] as List)
          .single['items'] as List;
      expect(items[0]['overview']['tags'], ['Work', 'client a']);
      expect(items[1]['overview']['tags'], isEmpty);

      final parsed = parseOnePux(bytes).records;
      expect(parsed[0].tags, ['Work', 'client a']);
      expect(parsed[1].tags, isEmpty);
    });

    test('import normalises, and tolerates a missing or odd tags value', () {
      expect(
        parseOnePux(_onePux({
          'title': 'Mail',
          'tags': ['#Work', 'work', 7, 'client   a'],
        })).records.single.tags,
        ['Work', 'client a'],
      );
      expect(parseOnePux(_onePux({'title': 'Mail'})).records.single.tags, isEmpty);
      expect(parseOnePux(_onePux({'title': 'Mail', 'tags': 'x'})).records.single.tags, isEmpty);
    });
  });
}
