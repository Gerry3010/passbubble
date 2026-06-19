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

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:passbubble/shared/widgets/share_link_dialog.dart';

Future<void> _pump(WidgetTester tester, Future<String> Function(Duration?) onCreate) {
  return tester.pumpWidget(
    MaterialApp(
      home: Scaffold(body: ShareLinkDialog(title: 'GitHub', onCreate: onCreate)),
    ),
  );
}

void main() {
  testWidgets('shows the expiry picker and a create button first', (tester) async {
    await _pump(tester, (_) async => 'https://x/web/#/share/t?k=k');
    expect(find.text('Create link'), findsOneWidget);
    expect(find.text('Expires after'), findsOneWidget);
    expect(find.textContaining('GitHub'), findsOneWidget);
  });

  testWidgets('passes the chosen validity and shows the resulting URL', (tester) async {
    Duration? captured;
    var called = false;
    await _pump(tester, (v) async {
      captured = v;
      called = true;
      return 'https://host/web/#/share/tok?k=secret';
    });

    // Default option is "7 days".
    await tester.tap(find.text('Create link'));
    await tester.pumpAndSettle();

    expect(called, isTrue);
    expect(captured, const Duration(days: 7));
    expect(find.textContaining('share/tok'), findsOneWidget);
    expect(find.text('Copy link'), findsOneWidget);
  });

  testWidgets('"Never" passes a null validity', (tester) async {
    Duration? captured = const Duration(days: 1);
    await _pump(tester, (v) async {
      captured = v;
      return 'https://host/web/#/share/tok?k=secret';
    });

    await tester.tap(find.text('Expires after'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Never').last);
    await tester.pumpAndSettle();
    await tester.tap(find.text('Create link'));
    await tester.pumpAndSettle();

    expect(captured, isNull);
  });
}
