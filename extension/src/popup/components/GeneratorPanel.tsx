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

import { useState } from 'react';
import browser from 'webextension-polyfill';
import type { GeneratorOptions } from '@passbubble/shared-ts';
import { GeneratorOptionsForm } from '../../shared/GeneratorOptionsForm.js';
import { GeneratorPreview } from '../../shared/GeneratorPreview.js';
import { useGenerator, useGeneratorDefaults } from '../../shared/useGenerator.js';
import { term, buttonPrimary, buttonGhost, link, muted, withDisabled } from '../../shared/theme.js';

// Generates locally (no server, no unlocked vault needed), starting from the
// stored defaults. Option changes here are not saved — the defaults are edited
// on the options page.
export function GeneratorPanel() {
  const defaults = useGeneratorDefaults();
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '10px' }}>
      <h3 style={{ fontSize: '14px', fontWeight: 700, margin: 0, color: term.green }}># password generator</h3>
      {defaults ? <GeneratorPanelBody defaults={defaults} /> : <p style={muted}>Loading…</p>}
    </div>
  );
}

function GeneratorPanelBody({ defaults }: { defaults: GeneratorOptions }) {
  const gen = useGenerator(defaults);
  const [copied, setCopied] = useState(false);

  async function copy() {
    if (!gen.password) return;
    await navigator.clipboard.writeText(gen.password);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  }

  const canCopy = !!gen.password && !gen.error;
  return (
    <>
      <GeneratorPreview password={gen.password} error={gen.error} onRegenerate={gen.regenerate} />
      <button
        type="button"
        onClick={() => void copy()}
        disabled={!canCopy}
        style={withDisabled(
          copied ? { ...buttonPrimary, padding: '6px', fontSize: '12px' } : { ...buttonGhost, padding: '6px', fontSize: '12px' },
          !canCopy,
        )}
      >
        {copied ? 'Copied!' : 'Copy to clipboard'}
      </button>
      <GeneratorOptionsForm value={gen.options} onChange={gen.setOptions} />
      <p style={{ ...muted, fontSize: '11px' }}>
        Changes here apply only to this password.{' '}
        <button type="button" onClick={() => void browser.runtime.openOptionsPage()} style={{ ...link, fontSize: '11px' }}>
          Change the defaults
        </button>
      </p>
    </>
  );
}
