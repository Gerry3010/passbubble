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

// Password health: strength heuristic (1:1 port of CheckStrength in
// cli/pkg/generator/generator.go — keep the implementations in sync), an
// HIBP k-anonymity breach check (only the first 5 SHA-1 hex chars ever leave
// the device) and the vault-wide report. Mirrors
// packages/shared-ts/src/health/ for the extension.

import 'dart:convert';

import 'package:cryptography/cryptography.dart';
import 'package:dio/dio.dart';

class StrengthResult {
  final int score;
  final String level;
  final List<String> feedback;
  const StrengthResult(
      {required this.score, required this.level, required this.feedback});
}

final _seqAlpha = RegExp(
    r'(abc|bcd|cde|def|efg|fgh|ghi|hij|ijk|jkl|klm|lmn|mno|nop|opq|pqr|qrs|rst|stu|tuv|uvw|vwx|wxy|xyz)');
final _seqDigit = RegExp(r'(012|123|234|345|456|567|678|789)');

StrengthResult checkStrength(String password) {
  final feedback = <String>[];
  final length = utf8.encode(password).length; // byte length, like Go's len()
  var score = 0;

  if (length >= 12) {
    score += 25;
  } else if (length >= 8) {
    score += 15;
  } else {
    feedback.add('Password is too short (minimum 8 characters)');
  }

  if (RegExp(r'[a-z]').hasMatch(password)) {
    score += 15;
  } else {
    feedback.add('Add lowercase letters');
  }
  if (RegExp(r'[A-Z]').hasMatch(password)) {
    score += 15;
  } else {
    feedback.add('Add uppercase letters');
  }
  if (RegExp(r'[0-9]').hasMatch(password)) {
    score += 15;
  } else {
    feedback.add('Add numbers');
  }
  if (RegExp(r'[^a-zA-Z0-9]').hasMatch(password)) {
    score += 20;
  } else {
    feedback.add('Add symbols');
  }

  var hasRepeated = false;
  for (var i = 0; i < password.length - 2; i++) {
    if (password[i] == password[i + 1] && password[i + 1] == password[i + 2]) {
      hasRepeated = true;
      break;
    }
  }
  if (hasRepeated) {
    score -= 10;
    feedback.add('Avoid repeated characters');
  }

  if (_seqAlpha.hasMatch(password.toLowerCase()) ||
      _seqDigit.hasMatch(password)) {
    score -= 15;
    feedback.add('Avoid sequential characters');
  }

  final String level;
  if (score >= 80) {
    level = 'Very Strong';
  } else if (score >= 65) {
    level = 'Strong';
  } else if (score >= 50) {
    level = 'Moderate';
  } else if (score >= 35) {
    level = 'Weak';
  } else {
    level = 'Very Weak';
  }
  return StrengthResult(score: score, level: level, feedback: feedback);
}

/// HIBP range client. Only sha1(password)[0..5) is sent; matching is local.
class HibpClient {
  final Dio _dio;
  final _rangeCache = <String, Map<String, int>>{};
  HibpClient([Dio? dio]) : _dio = dio ?? Dio();

  Future<int> pwnedCount(String password) async {
    final digest = await Sha1().hash(utf8.encode(password));
    final hex = digest.bytes
        .map((b) => b.toRadixString(16).padLeft(2, '0'))
        .join()
        .toUpperCase();
    final prefix = hex.substring(0, 5);
    final suffix = hex.substring(5);

    var range = _rangeCache[prefix];
    if (range == null) {
      final resp = await _dio.get<String>(
        'https://api.pwnedpasswords.com/range/$prefix',
        options: Options(
          responseType: ResponseType.plain,
          // Padding makes every response the same shape, so response sizes
          // leak nothing either.
          headers: {'Add-Padding': 'true'},
        ),
      );
      range = <String, int>{};
      for (final line in (resp.data ?? '').split('\n')) {
        final parts = line.trim().split(':');
        if (parts.length != 2) continue;
        final n = int.tryParse(parts[1]) ?? 0;
        if (n > 0) range[parts[0].toUpperCase()] = n; // padding rows are 0
      }
      _rangeCache[prefix] = range;
    }
    return range[suffix] ?? 0;
  }
}

class HealthFinding {
  final String id;
  final String name;
  final String detail;
  const HealthFinding(this.id, this.name, this.detail);
}

class HealthItemInput {
  final String id;
  final String name;
  final String password;
  final String updatedAt;
  const HealthItemInput({
    required this.id,
    required this.name,
    required this.password,
    this.updatedAt = '',
  });
}

class HealthReport {
  final int total;
  final int score;
  final List<HealthFinding> weak;
  final List<HealthFinding> reused;
  final List<HealthFinding> old;
  final List<HealthFinding> breached;
  final bool breachChecked;
  const HealthReport({
    required this.total,
    required this.score,
    required this.weak,
    required this.reused,
    required this.old,
    required this.breached,
    required this.breachChecked,
  });
}

Future<HealthReport> computeHealthReport(
  List<HealthItemInput> items, {
  bool checkBreaches = false,
  int weakThreshold = 50,
  int maxAgeDays = 365,
  HibpClient? hibp,
  DateTime? now,
}) async {
  final ref = now ?? DateTime.now();
  final withPassword = items.where((i) => i.password.isNotEmpty).toList();

  final weak = <HealthFinding>[];
  final old = <HealthFinding>[];
  final byPassword = <String, List<HealthItemInput>>{};

  for (final item in withPassword) {
    final strength = checkStrength(item.password);
    if (strength.score < weakThreshold) {
      weak.add(HealthFinding(item.id, item.name, 'score ${strength.score}'));
    }
    final changed = DateTime.tryParse(item.updatedAt);
    if (changed != null) {
      final ageDays = ref.difference(changed).inDays;
      if (ageDays > maxAgeDays) {
        old.add(HealthFinding(item.id, item.name, '$ageDays days'));
      }
    }
    byPassword.putIfAbsent(item.password, () => []).add(item);
  }

  final reused = <HealthFinding>[];
  for (final group in byPassword.values) {
    if (group.length < 2) continue;
    for (final item in group) {
      reused.add(
          HealthFinding(item.id, item.name, 'shared by ${group.length} entries'));
    }
  }

  final breached = <HealthFinding>[];
  var breachChecked = false;
  if (checkBreaches) {
    breachChecked = true;
    final client = hibp ?? HibpClient();
    for (final entry in byPassword.entries) {
      try {
        final count = await client.pwnedCount(entry.key);
        if (count > 0) {
          for (final item in entry.value) {
            breached
                .add(HealthFinding(item.id, item.name, '$count× in breaches'));
          }
        }
      } catch (_) {
        breachChecked = false; // offline/blocked — report partial result
        break;
      }
    }
  }

  final total = withPassword.length;
  var score = 100.0;
  if (total > 0) {
    score -= (40 * breached.length +
            30 * reused.length +
            25 * weak.length +
            10 * old.length) /
        total;
  }

  return HealthReport(
    total: total,
    score: score.clamp(0, 100).round(),
    weak: weak,
    reused: reused,
    old: old,
    breached: breached,
    breachChecked: breachChecked,
  );
}
