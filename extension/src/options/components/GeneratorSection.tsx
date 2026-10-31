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

import { useEffect, useState } from 'react';
import { DEFAULT_GENERATOR_OPTIONS, type GeneratorOptions } from '@passbubble/shared-ts';
import {
  readGeneratorDefaults,
  resetGeneratorDefaults,
  saveGeneratorDefaults,
} from '../../shared/generator-defaults.js';
import { GeneratorOptionsForm } from '../../shared/GeneratorOptionsForm.js';
import { GeneratorPreview } from '../../shared/GeneratorPreview.js';
import { useGenerator } from '../../shared/useGenerator.js';
import { term, buttonPrimary, buttonGhost, muted, errorText, withDisabled } from '../../shared/theme.js';

export function GeneratorSection() {
  const [saved, setSaved] = useState<GeneratorOptions | null>(null);

  useEffect(() => {
    void readGeneratorDefaults().then(setSaved);
  }, []);

  return (
    <section style={{ display: 'flex', flexDirection: 'column', gap: '12px', marginBottom: '32px' }}>
      <h2 style={{ fontSize: '18px', fontWeight: 700, color: term.green, fontFamily: term.font }}># password generator</h2>
      <p style={{ ...muted, fontSize: '13px' }}>
        Defaults for every password generator in the extension: the popup, the Generate button
        when adding an entry, and the box on signup pages. Changes made there apply only to that
        one password. Passwords are generated on this device; the server never sees them.
      </p>
      {saved && <GeneratorDefaultsEditor saved={saved} onSaved={setSaved} />}
    </section>
  );
}

function GeneratorDefaultsEditor({
  saved,
  onSaved,
}: {
  saved: GeneratorOptions;
  onSaved: (options: GeneratorOptions) => void;
}) {
  const gen = useGenerator(saved);
  const [status, setStatus] = useState<string | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);

  const dirty = (Object.keys(saved) as (keyof GeneratorOptions)[]).some((k) => gen.options[k] !== saved[k]);
  const canSave = dirty && !gen.error;

  function change(next: GeneratorOptions) {
    gen.setOptions(next);
    setStatus(null);
  }

  async function save() {
    setSaveError(null);
    try {
      await saveGeneratorDefaults(gen.options);
      onSaved(gen.options);
      setStatus('Saved.');
    } catch (err) {
      setSaveError(`Could not save: ${err instanceof Error ? err.message : String(err)}`);
    }
  }

  async function reset() {
    setSaveError(null);
    try {
      await resetGeneratorDefaults();
      const defaults = { ...DEFAULT_GENERATOR_OPTIONS };
      gen.setOptions(defaults);
      onSaved(defaults);
      setStatus('Reset to defaults.');
    } catch (err) {
      setSaveError(`Could not reset: ${err instanceof Error ? err.message : String(err)}`);
    }
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '12px', maxWidth: '420px' }}>
      <GeneratorOptionsForm value={gen.options} onChange={change} />
      <GeneratorPreview password={gen.password} error={gen.error} onRegenerate={gen.regenerate} />
      <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
        <button
          type="button"
          onClick={() => void save()}
          disabled={!canSave}
          style={withDisabled({ ...buttonPrimary, padding: '8px 16px' }, !canSave)}
        >
          Save
        </button>
        <button type="button" onClick={() => void reset()} style={{ ...buttonGhost, padding: '8px 16px' }}>
          Reset to defaults
        </button>
        {status && <span style={{ ...muted, fontSize: '12px' }}>{status}</span>}
      </div>
      {saveError && <p style={errorText}>{saveError}</p>}
    </div>
  );
}
