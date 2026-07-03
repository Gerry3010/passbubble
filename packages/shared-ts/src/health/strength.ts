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

// Password strength heuristic — a 1:1 port of CheckStrength in
// cli/pkg/generator/generator.go, so scores are identical across CLI,
// extension and app. Keep the two in sync.

export type StrengthLevel = 'Very Weak' | 'Weak' | 'Moderate' | 'Strong' | 'Very Strong';

export interface StrengthResult {
  score: number;
  level: StrengthLevel;
  length: number;
  hasLower: boolean;
  hasUpper: boolean;
  hasDigits: boolean;
  hasSymbols: boolean;
  feedback: string[];
}

const SEQ_ALPHA =
  /(abc|bcd|cde|def|efg|fgh|ghi|hij|ijk|jkl|klm|lmn|mno|nop|opq|pqr|qrs|rst|stu|tuv|uvw|vwx|wxy|xyz)/;
const SEQ_DIGIT = /(012|123|234|345|456|567|678|789)/;

export function checkStrength(password: string): StrengthResult {
  const feedback: string[] = [];
  // Byte length like Go's len() — the Go original counts UTF-8 bytes.
  const length = new TextEncoder().encode(password).length;
  let score = 0;

  if (length >= 12) score += 25;
  else if (length >= 8) score += 15;
  else feedback.push('Password is too short (minimum 8 characters)');

  const hasLower = /[a-z]/.test(password);
  if (hasLower) score += 15;
  else feedback.push('Add lowercase letters');

  const hasUpper = /[A-Z]/.test(password);
  if (hasUpper) score += 15;
  else feedback.push('Add uppercase letters');

  const hasDigits = /[0-9]/.test(password);
  if (hasDigits) score += 15;
  else feedback.push('Add numbers');

  const hasSymbols = /[^a-zA-Z0-9]/.test(password);
  if (hasSymbols) score += 20;
  else feedback.push('Add symbols');

  let hasRepeated = false;
  for (let i = 0; i < password.length - 2; i++) {
    if (password[i] === password[i + 1] && password[i + 1] === password[i + 2]) {
      hasRepeated = true;
      break;
    }
  }
  if (hasRepeated) {
    score -= 10;
    feedback.push('Avoid repeated characters');
  }

  if (SEQ_ALPHA.test(password.toLowerCase()) || SEQ_DIGIT.test(password)) {
    score -= 15;
    feedback.push('Avoid sequential characters');
  }

  let level: StrengthLevel;
  if (score >= 80) level = 'Very Strong';
  else if (score >= 65) level = 'Strong';
  else if (score >= 50) level = 'Moderate';
  else if (score >= 35) level = 'Weak';
  else level = 'Very Weak';

  return { score, level, length, hasLower, hasUpper, hasDigits, hasSymbols, feedback };
}
