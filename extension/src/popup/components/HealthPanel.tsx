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

// Vault-wide password health. The report is computed in the background
// (HEALTH_REPORT) — this panel only ever sees ids, names and category data.

import { useState } from 'react';
import browser from 'webextension-polyfill';
import type { HealthReport } from '@passbubble/shared-ts';
import { MessageType } from '../../shared/constants.js';
import { term, buttonPrimary, muted, errorText, withDisabled } from '../../shared/theme.js';

export function HealthPanel() {
  const [report, setReport] = useState<HealthReport | null>(null);
  const [checkBreaches, setCheckBreaches] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function run() {
    setBusy(true);
    setError(null);
    try {
      const resp = (await browser.runtime.sendMessage({
        type: MessageType.HEALTH_REPORT,
        payload: { checkBreaches },
      })) as { report?: HealthReport; locked?: boolean };
      if (resp.locked || !resp.report) {
        setError('Vault is locked');
        return;
      }
      setReport(resp.report);
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }

  const scoreColor = (s: number) => (s >= 80 ? term.green : s >= 50 ? term.amber : term.red);

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
      <label style={{ display: 'flex', alignItems: 'center', gap: '6px', fontSize: '12px', color: term.muted }}>
        <input
          type="checkbox"
          checked={checkBreaches}
          onChange={(e) => setCheckBreaches(e.target.checked)}
        />
        Check known breaches (HIBP)
      </label>
      <button onClick={() => void run()} disabled={busy} style={withDisabled(buttonPrimary, busy)}>
        {busy ? 'Analyzing…' : report ? 'Re-run health check' : 'Run health check'}
      </button>
      <span style={{ color: term.muted, fontSize: '10px' }}>
        Analysis runs locally. The breach check uses k-anonymity — your passwords never leave this
        device.
      </span>

      {error && <p style={errorText}>{error}</p>}

      {report && (
        <>
          <div
            style={{
              border: `1px solid ${term.border}`,
              background: term.surface,
              borderRadius: '4px',
              padding: '10px',
              display: 'flex',
              justifyContent: 'space-between',
              alignItems: 'baseline',
            }}
          >
            <span style={{ color: term.muted, fontSize: '12px' }}>
              vault score ({report.total} passwords)
            </span>
            <span style={{ color: scoreColor(report.score), fontSize: '22px', fontWeight: 700 }}>
              {report.score}
            </span>
          </div>

          <Section
            title={`breached (${report.breached.length})`}
            empty={report.breachChecked ? 'none found' : 'not checked'}
            items={report.breached.map((f) => [f.name, `${f.breachCount}× in breaches`])}
            color={term.red}
          />
          <Section
            title={`reused (${report.reused.length})`}
            empty="none"
            items={report.reused.map((f) => [f.name, `shared by ${f.reusedWith} entries`])}
            color={term.amber}
          />
          <Section
            title={`weak (${report.weak.length})`}
            empty="none"
            items={report.weak.map((f) => [f.name, `score ${f.score}`])}
            color={term.amber}
          />
          <Section
            title={`old (${report.old.length})`}
            empty="none"
            items={report.old.map((f) => [f.name, `${f.ageDays} days`])}
            color={term.muted}
          />
        </>
      )}
      {!report && !busy && <p style={muted}>Analyze your vault for weak, reused, old and breached passwords.</p>}
    </div>
  );
}

function Section({
  title,
  empty,
  items,
  color,
}: {
  title: string;
  empty: string;
  items: [string, string][];
  color: string;
}) {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '4px' }}>
      <span style={{ color, fontSize: '12px', fontWeight: 700 }}># {title}</span>
      {items.length === 0 ? (
        <span style={{ color: term.muted, fontSize: '11px', paddingLeft: '8px' }}>{empty}</span>
      ) : (
        items.map(([name, detail], i) => (
          <div
            key={i}
            style={{
              display: 'flex',
              justifyContent: 'space-between',
              gap: '8px',
              border: `1px solid ${term.border}`,
              background: term.surface,
              borderRadius: '4px',
              padding: '5px 8px',
              fontSize: '12px',
            }}
          >
            <span
              style={{ color: term.green, overflow: 'hidden', whiteSpace: 'nowrap', textOverflow: 'ellipsis' }}
            >
              {name}
            </span>
            <span style={{ color: term.muted, whiteSpace: 'nowrap', fontSize: '11px' }}>{detail}</span>
          </div>
        ))
      )}
    </div>
  );
}
