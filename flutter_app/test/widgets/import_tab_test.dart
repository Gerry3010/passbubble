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

import 'dart:typed_data';

import 'package:file_picker/file_picker.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:passbubble/core/jobs/job_runner.dart';
import 'package:passbubble/core/jobs/local_job.dart';
import 'package:passbubble/features/manage/import_tab.dart';
import 'package:passbubble/shared/widgets/pb_button.dart';

class _FakePicker extends FilePicker {
  @override
  Future<FilePickerResult?> pickFiles({
    String? dialogTitle,
    String? initialDirectory,
    FileType type = FileType.any,
    List<String>? allowedExtensions,
    Function(FilePickerStatus)? onFileLoading,
    bool allowCompression = false,
    int compressionQuality = 0,
    bool allowMultiple = false,
    bool withData = false,
    bool withReadStream = false,
    bool lockParentWindow = false,
    bool readSequential = false,
  }) async =>
      FilePickerResult([PlatformFile(name: 'a.csv', size: 1, bytes: Uint8List(1))]);
}

class _RecordingRunner extends JobRunner {
  final calls = <({DupStrategy dup, OverwriteMode mode})>[];

  @override
  Future<void> startImport({
    required Uint8List bytes,
    required ImportFormat format,
    required DupStrategy dup,
    OverwriteMode overwriteMode = OverwriteMode.merge,
  }) async =>
      calls.add((dup: dup, mode: overwriteMode));
}

void main() {
  final checkbox = find.byKey(const ValueKey('overwrite-completely'));

  setUp(() => FilePicker.platform = _FakePicker());

  Future<_RecordingRunner> pumpTab(WidgetTester tester) async {
    final runner = _RecordingRunner();
    await tester.pumpWidget(ProviderScope(
      overrides: [jobRunnerProvider.overrideWith(() => runner)],
      child: const MaterialApp(
        home: DefaultTabController(length: 4, child: Scaffold(body: ImportTab())),
      ),
    ));
    await tester.pumpAndSettle();
    return runner;
  }

  Future<void> choose(WidgetTester tester, String strategy) async {
    await tester.tap(find.text(strategy));
    await tester.pumpAndSettle();
  }

  Future<void> startImport(WidgetTester tester) async {
    await tester.tap(find.byType(PbButton));
    await tester.pumpAndSettle();
  }

  bool isChecked(WidgetTester tester) => tester.widget<CheckboxListTile>(checkbox).value!;

  testWidgets('the option shows only for overwrite, off by default, with its text',
      (tester) async {
    await pumpTab(tester);
    expect(checkbox, findsNothing);

    await choose(tester, 'Overwrite');
    expect(checkbox, findsOneWidget);
    expect(isChecked(tester), isFalse);
    expect(find.text('Overwrite completely'), findsOneWidget);
    expect(
        find.text('Fields missing from the file are removed from the entry. '
            'Off: only values in the file replace existing ones.'),
        findsOneWidget);

    await choose(tester, 'Skip');
    expect(checkbox, findsNothing);
  });

  testWidgets('the mode reaches the job', (tester) async {
    final runner = await pumpTab(tester);

    await choose(tester, 'Overwrite');
    await startImport(tester);
    expect(runner.calls.last, (dup: DupStrategy.overwrite, mode: OverwriteMode.merge));

    await tester.tap(checkbox);
    await tester.pumpAndSettle();
    expect(isChecked(tester), isTrue);
    await startImport(tester);
    expect(runner.calls.last, (dup: DupStrategy.overwrite, mode: OverwriteMode.complete));
  });

  testWidgets('leaving overwrite switches it off again', (tester) async {
    final runner = await pumpTab(tester);

    await choose(tester, 'Overwrite');
    await tester.tap(checkbox);
    await tester.pumpAndSettle();

    await choose(tester, 'Skip');
    await startImport(tester);
    expect(runner.calls.last, (dup: DupStrategy.skip, mode: OverwriteMode.merge));

    await choose(tester, 'Overwrite');
    expect(isChecked(tester), isFalse);
  });
}
