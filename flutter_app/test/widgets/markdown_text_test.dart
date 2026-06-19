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
import 'package:passbubble/shared/widgets/markdown_text.dart';

Future<void> _pump(WidgetTester tester, String md) async {
  await tester.pumpWidget(
    MaterialApp(home: Scaffold(body: SingleChildScrollView(child: MarkdownText(md)))),
  );
}

void main() {
  testWidgets('renders headings, bullets and paragraphs as separate spans', (tester) async {
    await _pump(tester, '## What\'s New\n\n- First item\n- Second item\n\nPlain paragraph.');

    expect(find.textContaining("What's New"), findsOneWidget);
    expect(find.textContaining('First item'), findsOneWidget);
    expect(find.textContaining('Second item'), findsOneWidget);
    expect(find.textContaining('Plain paragraph.'), findsOneWidget);
    // bullets render a marker
    expect(find.textContaining('•'), findsWidgets);
  });

  testWidgets('does not render raw markdown markers for bold/code', (tester) async {
    await _pump(tester, 'A **bold** and `code` word.');
    // The literal "**" / backticks must not appear in the rendered text.
    expect(find.textContaining('**'), findsNothing);
    expect(find.textContaining('`'), findsNothing);
    expect(find.textContaining('bold'), findsOneWidget);
    expect(find.textContaining('code'), findsOneWidget);
  });

  testWidgets('renders a link label without the markdown link syntax', (tester) async {
    await _pump(tester, 'See [the changelog](https://example.com/CHANGELOG.md) for details.');
    expect(find.textContaining('the changelog'), findsOneWidget);
    expect(find.textContaining(']('), findsNothing);
  });
}
