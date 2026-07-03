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

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { checkStrength } from '../strength.js';
import { pwnedCount, clearHibpCache } from '../hibp.js';
import { computeHealthReport } from '../report.js';

describe('checkStrength', () => {
  // Same vectors and expected ranges as TestCheckStrength in
  // cli/pkg/generator/generator_test.go — scores must stay consistent.
  const vectors: Array<[string, number, number]> = [
    ['12345678', 0, 35],
    ['password', 15, 50],
    ['Password1', 35, 65],
    ['Password1!', 65, 85],
    ['P@ssw0rd!2023', 80, 100],
  ];

  it.each(vectors)('scores %s within [%d, %d]', (password, min, max) => {
    const r = checkStrength(password);
    expect(r.score).toBeGreaterThanOrEqual(min);
    expect(r.score).toBeLessThanOrEqual(max);
    expect(r.length).toBe(password.length);
  });

  it('flags repeated and sequential characters', () => {
    expect(checkStrength('aaaBBB111!!!').feedback).toContain('Avoid repeated characters');
    expect(checkStrength('abcXYZ9$q').feedback).toContain('Avoid sequential characters');
  });
});

describe('pwnedCount', () => {
  const fetchMock = vi.fn();

  beforeEach(() => {
    clearHibpCache();
    vi.stubGlobal('fetch', fetchMock);
    fetchMock.mockReset();
  });

  afterEach(() => vi.unstubAllGlobals());

  it('sends only the 5-char SHA-1 prefix and matches the suffix locally', async () => {
    // sha1('password') = 5BAA61E4C9B93F3F0682250B6CF8331B7EE68FD8
    fetchMock.mockResolvedValue({
      ok: true,
      text: async () => '1E4C9B93F3F0682250B6CF8331B7EE68FD8:3730471\nAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA:0\n',
    });

    const count = await pwnedCount('password');

    expect(count).toBe(3730471);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const url = fetchMock.mock.calls[0][0] as string;
    expect(url).toBe('https://api.pwnedpasswords.com/range/5BAA6');
    // Nothing beyond the 5-char prefix may appear in the request.
    expect(url).not.toContain('1E4C9B93');
  });

  it('returns 0 for passwords not in the list', async () => {
    fetchMock.mockResolvedValue({ ok: true, text: async () => 'SOMEOTHERSUFFIX:12\n' });
    expect(await pwnedCount('password')).toBe(0);
  });

  it('caches range responses per prefix', async () => {
    fetchMock.mockResolvedValue({ ok: true, text: async () => 'X:1\n' });
    await pwnedCount('password');
    await pwnedCount('password');
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });
});

describe('computeHealthReport', () => {
  const now = Date.parse('2026-07-03T00:00:00Z');

  it('categorises weak, reused and old passwords', async () => {
    const report = await computeHealthReport(
      [
        { id: '1', name: 'weak-entry', password: 'abc', updatedAt: '2026-06-01T00:00:00Z' },
        { id: '2', name: 'reused-a', password: 'Sh4red!Secret42x', updatedAt: '2026-06-01T00:00:00Z' },
        { id: '3', name: 'reused-b', password: 'Sh4red!Secret42x', updatedAt: '2026-06-01T00:00:00Z' },
        { id: '4', name: 'old-entry', password: 'G00d&Strong#Pass', updatedAt: '2024-01-01T00:00:00Z' },
        { id: '5', name: 'fine', password: 'T0tally-F1ne!Pass', updatedAt: '2026-06-01T00:00:00Z' },
        { id: '6', name: 'no-password', password: '' },
      ],
      { now },
    );

    expect(report.total).toBe(5); // empty-password entry not counted
    expect(report.weak.map((f) => f.id)).toEqual(['1']);
    expect(report.reused.map((f) => f.id).sort()).toEqual(['2', '3']);
    expect(report.reused[0].reusedWith).toBe(2);
    expect(report.old.map((f) => f.id)).toEqual(['4']);
    expect(report.old[0].ageDays).toBeGreaterThan(365);
    expect(report.breachChecked).toBe(false);
    expect(report.breached).toEqual([]);
    expect(report.score).toBeLessThan(100);
  });

  it('gives a clean vault a perfect score', async () => {
    const report = await computeHealthReport(
      [{ id: '1', name: 'fine', password: 'T0tally-F1ne!Pass', updatedAt: '2026-06-01T00:00:00Z' }],
      { now },
    );
    expect(report.score).toBe(100);
    expect(report.weak).toEqual([]);
    expect(report.reused).toEqual([]);
    expect(report.old).toEqual([]);
  });
});
