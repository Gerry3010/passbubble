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

import '../../core/theme/app_theme.dart';

/// App-bar / screen title in the brand's shell-prompt style:
/// a muted `passbubble:~$ ` prefix followed by the green screen name.
/// See docs/design-guidelines.md (§3).
class PromptTitle extends StatelessWidget {
  /// The action/screen word(s), lowercase — e.g. `vault`, `settings`,
  /// `new entry`. Rendered after the `passbubble:~$ ` prompt.
  final String screen;

  const PromptTitle(this.screen, {super.key});

  @override
  Widget build(BuildContext context) {
    final base = Theme.of(context).appBarTheme.titleTextStyle;
    return Text.rich(
      TextSpan(
        children: [
          const TextSpan(
            text: 'passbubble:~\$ ',
            style: TextStyle(color: AppTheme.onBgDim),
          ),
          TextSpan(text: screen, style: const TextStyle(color: AppTheme.green)),
        ],
      ),
      style: base,
      maxLines: 1,
      overflow: TextOverflow.ellipsis,
    );
  }
}
