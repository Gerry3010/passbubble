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
import 'package:passbubble/core/health/health_service.dart';

void main() {
  group('checkStrength', () {
    // Same vectors and ranges as TestCheckStrength in
    // cli/pkg/generator/generator_test.go and shared-ts health.test.ts.
    const vectors = <(String, int, int)>[
      ('12345678', 0, 35),
      ('password', 15, 50),
      ('Password1', 35, 65),
      ('Password1!', 65, 85),
      ('P@ssw0rd!2023', 80, 100),
    ];

    for (final (password, min, max) in vectors) {
      test('scores $password within [$min, $max]', () {
        final r = checkStrength(password);
        expect(r.score, greaterThanOrEqualTo(min));
        expect(r.score, lessThanOrEqualTo(max));
      });
    }

    test('flags repeated and sequential characters', () {
      expect(checkStrength('aaaBBB111!!!').feedback,
          contains('Avoid repeated characters'));
      expect(checkStrength('abcXYZ9\$q').feedback,
          contains('Avoid sequential characters'));
    });
  });

  group('computeHealthReport', () {
    final now = DateTime.parse('2026-07-03T00:00:00Z');

    test('categorises weak, reused and old passwords', () async {
      final report = await computeHealthReport(
        [
          const HealthItemInput(
              id: '1',
              name: 'weak-entry',
              password: 'abc',
              updatedAt: '2026-06-01T00:00:00Z'),
          const HealthItemInput(
              id: '2',
              name: 'reused-a',
              password: 'Sh4red!Secret42x',
              updatedAt: '2026-06-01T00:00:00Z'),
          const HealthItemInput(
              id: '3',
              name: 'reused-b',
              password: 'Sh4red!Secret42x',
              updatedAt: '2026-06-01T00:00:00Z'),
          const HealthItemInput(
              id: '4',
              name: 'old-entry',
              password: 'G00d&Strong#Pass',
              updatedAt: '2024-01-01T00:00:00Z'),
          const HealthItemInput(
              id: '5',
              name: 'fine',
              password: 'T0tally-F1ne!Pass',
              updatedAt: '2026-06-01T00:00:00Z'),
          const HealthItemInput(id: '6', name: 'no-password', password: ''),
        ],
        now: now,
      );

      expect(report.total, 5);
      expect(report.weak.map((f) => f.id), ['1']);
      expect(report.reused.map((f) => f.id).toList()..sort(), ['2', '3']);
      expect(report.old.map((f) => f.id), ['4']);
      expect(report.breachChecked, isFalse);
      expect(report.breached, isEmpty);
      expect(report.score, lessThan(100));
    });

    test('gives a clean vault a perfect score', () async {
      final report = await computeHealthReport(
        [
          const HealthItemInput(
              id: '1',
              name: 'fine',
              password: 'T0tally-F1ne!Pass',
              updatedAt: '2026-06-01T00:00:00Z'),
        ],
        now: now,
      );
      expect(report.score, 100);
      expect(report.weak, isEmpty);
      expect(report.reused, isEmpty);
      expect(report.old, isEmpty);
    });
  });
}
