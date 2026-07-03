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

// Vault-wide password health report: weak, reused, old, breached. The caller
// supplies already-decrypted passwords; nothing here stores or transmits them
// (HIBP uses the k-anonymity range API — see hibp.ts).

import { checkStrength } from './strength.js';
import { pwnedCount } from './hibp.js';

export interface HealthItemInput {
  id: string;
  name: string;
  password: string;
  /** ISO timestamp of the last change (entry updated_at). */
  updatedAt?: string;
}

export interface HealthFinding {
  id: string;
  name: string;
  /** Strength score 0–100 (weak findings). */
  score?: number;
  /** Number of entries sharing this password (reused findings). */
  reusedWith?: number;
  /** Days since the password last changed (old findings). */
  ageDays?: number;
  /** Occurrences in known breaches (breached findings). */
  breachCount?: number;
}

export interface HealthReport {
  total: number;
  /** Overall vault score 0–100 (100 = no findings). */
  score: number;
  weak: HealthFinding[];
  reused: HealthFinding[];
  old: HealthFinding[];
  breached: HealthFinding[];
  /** True when the HIBP check ran (opt-in and network-dependent). */
  breachChecked: boolean;
}

export interface HealthOptions {
  /** Run the HIBP k-anonymity breach check (network). Default false. */
  checkBreaches?: boolean;
  /** Passwords below this strength score count as weak. Default 50. */
  weakThreshold?: number;
  /** Passwords older than this count as old. Default 365 days. */
  maxAgeDays?: number;
  /** Reference time (ms) for age computation — injectable for tests. */
  now?: number;
}

export async function computeHealthReport(
  items: HealthItemInput[],
  opts: HealthOptions = {},
): Promise<HealthReport> {
  const weakThreshold = opts.weakThreshold ?? 50;
  const maxAgeDays = opts.maxAgeDays ?? 365;
  const now = opts.now ?? Date.now();

  const withPassword = items.filter((i) => i.password);

  const weak: HealthFinding[] = [];
  const old: HealthFinding[] = [];
  const byPassword = new Map<string, HealthItemInput[]>();

  for (const item of withPassword) {
    const strength = checkStrength(item.password);
    if (strength.score < weakThreshold) {
      weak.push({ id: item.id, name: item.name, score: strength.score });
    }

    const changed = item.updatedAt ? Date.parse(item.updatedAt) : NaN;
    if (!Number.isNaN(changed)) {
      const ageDays = Math.floor((now - changed) / 86_400_000);
      if (ageDays > maxAgeDays) old.push({ id: item.id, name: item.name, ageDays });
    }

    const group = byPassword.get(item.password);
    if (group) group.push(item);
    else byPassword.set(item.password, [item]);
  }

  const reused: HealthFinding[] = [];
  for (const group of byPassword.values()) {
    if (group.length < 2) continue;
    for (const item of group) {
      reused.push({ id: item.id, name: item.name, reusedWith: group.length });
    }
  }

  const breached: HealthFinding[] = [];
  let breachChecked = false;
  if (opts.checkBreaches) {
    breachChecked = true;
    // One check per distinct password; the range cache dedupes prefixes.
    for (const [password, group] of byPassword) {
      let count: number;
      try {
        count = await pwnedCount(password);
      } catch {
        breachChecked = false; // offline / blocked — report partial result
        break;
      }
      if (count > 0) {
        for (const item of group) {
          breached.push({ id: item.id, name: item.name, breachCount: count });
        }
      }
    }
  }

  // Vault score: start at 100, subtract per finding relative to vault size.
  // Breached and reused hurt most; capped at 0.
  const total = withPassword.length;
  let score = 100;
  if (total > 0) {
    score -=
      (40 * breached.length + 30 * reused.length + 25 * weak.length + 10 * old.length) / total;
  }
  score = Math.max(0, Math.min(100, Math.round(score)));

  const bySeverity = (a: HealthFinding, b: HealthFinding) => a.name.localeCompare(b.name);
  weak.sort((a, b) => (a.score ?? 0) - (b.score ?? 0));
  reused.sort(bySeverity);
  old.sort((a, b) => (b.ageDays ?? 0) - (a.ageDays ?? 0));
  breached.sort((a, b) => (b.breachCount ?? 0) - (a.breachCount ?? 0));

  return { total, score, weak, reused, old, breached, breachChecked };
}
