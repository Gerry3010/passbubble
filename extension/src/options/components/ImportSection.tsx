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

import { useRef, useState } from 'react';
import browser from 'webextension-polyfill';
import { MessageType } from '../../shared/constants.js';
import { parsePsono, type ImportParseResult } from '../../shared/psono-import.js';
import { term, buttonPrimary, buttonGhost, muted, errorText, withDisabled } from '../../shared/theme.js';

type Phase = 'idle' | 'parsed' | 'importing' | 'done';

interface Progress {
  total: number;
  created: number;
  failed: number;
}

export function ImportSection() {
  const fileInput = useRef<HTMLInputElement>(null);
  const [parsed, setParsed] = useState<ImportParseResult | null>(null);
  const [phase, setPhase] = useState<Phase>('idle');
  const [error, setError] = useState<string | null>(null);
  const [progress, setProgress] = useState<Progress>({ total: 0, created: 0, failed: 0 });

  function reset() {
    setParsed(null);
    setPhase('idle');
    setError(null);
    setProgress({ total: 0, created: 0, failed: 0 });
    if (fileInput.current) fileInput.current.value = '';
  }

  async function onFile(e: React.ChangeEvent<HTMLInputElement>) {
    setError(null);
    const file = e.target.files?.[0];
    if (!file) return;
    try {
      const text = await file.text();
      const result = parsePsono(text);
      if (result.records.length === 0) {
        setError('No importable entries found in this file.');
        setParsed(null);
        setPhase('idle');
        return;
      }
      setParsed(result);
      setPhase('parsed');
    } catch {
      setError('Could not parse the file — is it an unencrypted Psono JSON export?');
      setParsed(null);
      setPhase('idle');
    }
  }

  async function runImport() {
    if (!parsed) return;
    setError(null);
    setPhase('importing');
    let created = 0;
    let failed = 0;
    setProgress({ total: parsed.records.length, created, failed });

    for (const rec of parsed.records) {
      try {
        const resp = (await browser.runtime.sendMessage({
          type: MessageType.CREATE_ENTRY,
          payload: {
            name: rec.name,
            type: rec.type,
            url: rec.url,
            matchPatterns: rec.matchPatterns,
            data: rec.data,
          },
        })) as { id?: string; locked?: boolean };
        if (resp?.locked) {
          setError('Vault is locked. Open the extension popup and unlock, then import again.');
          setPhase('parsed');
          return;
        }
        if (resp?.id) created++;
        else failed++;
      } catch {
        failed++;
      }
      setProgress({ total: parsed.records.length, created, failed });
    }
    setPhase('done');
  }

  return (
    <section style={{ display: 'flex', flexDirection: 'column', gap: '12px', marginBottom: '32px' }}>
      <h2 style={{ fontSize: '18px', fontWeight: 700, color: term.green, fontFamily: term.font }}># import</h2>
      <p style={{ ...muted, fontSize: '13px' }}>
        Import an <strong>unencrypted Psono JSON</strong> export. URL filters become autofill match
        patterns. Entries are added at the root (folders are not recreated — use the CLI to preserve
        folder structure).
      </p>

      {error && <p style={errorText}>{error}</p>}

      <input
        ref={fileInput}
        type="file"
        accept="application/json,.json"
        onChange={(e) => void onFile(e)}
        disabled={phase === 'importing'}
        style={{ color: term.muted, fontSize: '13px', fontFamily: term.font }}
      />

      {parsed && phase !== 'done' && (
        <p style={{ ...muted, fontSize: '13px' }}>
          <span style={{ color: term.green }}>{parsed.records.length}</span> entr
          {parsed.records.length === 1 ? 'y' : 'ies'} ready
          {parsed.skipped > 0 ? `, ${parsed.skipped} skipped` : ''}.
        </p>
      )}

      {phase === 'parsed' && (
        <button
          onClick={() => void runImport()}
          style={{ ...buttonPrimary, alignSelf: 'flex-start', padding: '8px 16px' }}
        >
          Import {parsed?.records.length} entries
        </button>
      )}

      {phase === 'importing' && (
        <p style={{ ...muted, fontSize: '13px' }}>
          Importing… {progress.created + progress.failed}/{progress.total}
        </p>
      )}

      {phase === 'done' && (
        <>
          <p style={{ ...muted, fontSize: '13px' }}>
            Done: <span style={{ color: term.green }}>{progress.created} created</span>
            {progress.failed > 0 ? `, ${progress.failed} failed` : ''}.
          </p>
          <button
            onClick={reset}
            style={withDisabled({ ...buttonGhost, alignSelf: 'flex-start', padding: '8px 16px' }, false)}
          >
            Import another file
          </button>
        </>
      )}
    </section>
  );
}
