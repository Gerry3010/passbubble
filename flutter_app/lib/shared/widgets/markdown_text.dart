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

import 'package:flutter/gestures.dart';
import 'package:flutter/material.dart';
import 'package:url_launcher/url_launcher.dart';

import '../../core/theme/app_theme.dart';

/// Lightweight Markdown renderer for release notes / changelog content. Supports
/// the elements that actually appear there: ATX headings (`#`–`######`), bullet
/// lists (`-`/`*`, nestable by indentation), `**bold**`, `` `code` `` and
/// tappable `[label](url)` links. Anything else is rendered as plain text.
class MarkdownText extends StatefulWidget {
  final String data;
  const MarkdownText(this.data, {super.key});

  @override
  State<MarkdownText> createState() => _MarkdownTextState();
}

class _MarkdownTextState extends State<MarkdownText> {
  final List<TapGestureRecognizer> _recognizers = [];

  @override
  void dispose() {
    _disposeRecognizers();
    super.dispose();
  }

  void _disposeRecognizers() {
    for (final r in _recognizers) {
      r.dispose();
    }
    _recognizers.clear();
  }

  static final _heading = RegExp(r'^(#{1,6})\s+(.*)$');
  static final _bullet = RegExp(r'^(\s*)[-*]\s+(.*)$');
  static final _inline = RegExp(
    r'(\*\*([^*]+)\*\*)|(`([^`]+)`)|(\[([^\]]+)\]\(([^)\s]+)\))',
  );

  @override
  Widget build(BuildContext context) {
    _disposeRecognizers(); // recreated below for the current build
    final theme = Theme.of(context);
    final lines = widget.data.replaceAll('\r\n', '\n').split('\n');
    final children = <Widget>[];

    for (final raw in lines) {
      final line = raw.trimRight();
      if (line.trim().isEmpty) {
        children.add(const SizedBox(height: 8));
        continue;
      }

      final h = _heading.firstMatch(line);
      if (h != null) {
        final level = h.group(1)!.length;
        final style = (level <= 1
                ? theme.textTheme.titleLarge
                : level == 2
                    ? theme.textTheme.titleMedium
                    : theme.textTheme.titleSmall)
            ?.copyWith(color: AppTheme.green);
        children.add(Padding(
          padding: const EdgeInsets.only(top: 10, bottom: 4),
          child: Text.rich(_spans(h.group(2)!, style)),
        ));
        continue;
      }

      final b = _bullet.firstMatch(line);
      if (b != null) {
        final indent = b.group(1)!.length;
        children.add(Padding(
          padding: EdgeInsets.only(left: 8 + indent.toDouble(), top: 2, bottom: 2),
          child: Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text('•  ', style: theme.textTheme.bodySmall),
              Expanded(child: Text.rich(_spans(b.group(2)!, theme.textTheme.bodySmall))),
            ],
          ),
        ));
        continue;
      }

      children.add(Padding(
        padding: const EdgeInsets.symmetric(vertical: 2),
        child: Text.rich(_spans(line, theme.textTheme.bodySmall?.copyWith(height: 1.4))),
      ));
    }

    return Column(crossAxisAlignment: CrossAxisAlignment.start, children: children);
  }

  TextSpan _spans(String text, TextStyle? base) {
    final spans = <InlineSpan>[];
    var last = 0;
    for (final m in _inline.allMatches(text)) {
      if (m.start > last) {
        spans.add(TextSpan(text: text.substring(last, m.start), style: base));
      }
      if (m.group(1) != null) {
        spans.add(TextSpan(
          text: m.group(2),
          style: base?.copyWith(fontWeight: FontWeight.bold),
        ));
      } else if (m.group(3) != null) {
        spans.add(TextSpan(
          text: m.group(4),
          style: base?.copyWith(
            fontFamily: 'monospace',
            backgroundColor: AppTheme.green.withValues(alpha: 0.12),
          ),
        ));
      } else if (m.group(5) != null) {
        final url = m.group(7)!;
        final recognizer = TapGestureRecognizer()
          ..onTap = () => launchUrl(Uri.parse(url), mode: LaunchMode.externalApplication);
        _recognizers.add(recognizer);
        spans.add(TextSpan(
          text: m.group(6),
          style: base?.copyWith(
            color: AppTheme.green,
            decoration: TextDecoration.underline,
          ),
          recognizer: recognizer,
        ));
      }
      last = m.end;
    }
    if (last < text.length) {
      spans.add(TextSpan(text: text.substring(last), style: base));
    }
    return TextSpan(children: spans);
  }
}
