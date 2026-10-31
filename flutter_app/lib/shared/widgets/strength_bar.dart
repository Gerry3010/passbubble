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

import '../../core/health/health_service.dart';
import '../../core/theme/app_theme.dart';

/// Thin strength bar with the level, rated locally by [checkStrength] (the
/// same heuristic as password health and the other clients).
class StrengthBar extends StatelessWidget {
  final String password;
  const StrengthBar({super.key, required this.password});

  @override
  Widget build(BuildContext context) {
    final strength = checkStrength(password);
    final score = strength.score;
    final color = score >= 80
        ? AppTheme.green
        : score >= 50
            ? AppTheme.amber
            : AppTheme.error;

    return Row(
      children: [
        Expanded(
          child: LinearProgressIndicator(
            value: (score / 100).clamp(0.0, 1.0),
            backgroundColor: AppTheme.border,
            valueColor: AlwaysStoppedAnimation(color),
            minHeight: 3,
          ),
        ),
        const SizedBox(width: 8),
        Text(strength.level, style: TextStyle(color: color, fontSize: 11)),
      ],
    );
  }
}
