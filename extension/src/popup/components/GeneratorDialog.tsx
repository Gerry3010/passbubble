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

// Overlay behind a "Generate" button: preview, options, Cancel and OK. It
// starts from the stored defaults; option changes apply to this dialog only.
// Only OK hands the password to the caller.

import type { GeneratorOptions } from '@passbubble/shared-ts';
import { GeneratorOptionsForm } from '../../shared/GeneratorOptionsForm.js';
import { GeneratorPreview } from '../../shared/GeneratorPreview.js';
import { useGenerator, useGeneratorDefaults } from '../../shared/useGenerator.js';
import { term, buttonPrimary, buttonGhost, muted, withDisabled } from '../../shared/theme.js';

export function GeneratorDialog({
  onCancel,
  onConfirm,
}: {
  onCancel: () => void;
  onConfirm: (password: string) => void;
}) {
  const defaults = useGeneratorDefaults();
  return (
    <div
      style={{
        position: 'fixed',
        inset: 0,
        zIndex: 10,
        background: 'rgba(0, 0, 0, 0.75)',
        display: 'flex',
        alignItems: 'flex-start',
        justifyContent: 'center',
        padding: '12px',
        overflowY: 'auto',
      }}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-label="Generate password"
        onKeyDown={(e) => {
          if (e.key === 'Escape') onCancel();
        }}
        style={{
          width: '100%',
          display: 'flex',
          flexDirection: 'column',
          gap: '10px',
          padding: '12px',
          background: term.bg,
          border: `1px solid ${term.green}`,
          borderRadius: '4px',
        }}
      >
        <h3 style={{ margin: 0, fontSize: '14px', color: term.green }}># generate password</h3>
        {defaults ? (
          <GeneratorDialogBody defaults={defaults} onCancel={onCancel} onConfirm={onConfirm} />
        ) : (
          <p style={muted}>Loading…</p>
        )}
      </div>
    </div>
  );
}

function GeneratorDialogBody({
  defaults,
  onCancel,
  onConfirm,
}: {
  defaults: GeneratorOptions;
  onCancel: () => void;
  onConfirm: (password: string) => void;
}) {
  const gen = useGenerator(defaults);
  const canConfirm = !!gen.password && !gen.error;
  return (
    <>
      <GeneratorPreview password={gen.password} error={gen.error} onRegenerate={gen.regenerate} />
      <GeneratorOptionsForm value={gen.options} onChange={gen.setOptions} />
      <div style={{ display: 'flex', gap: '6px' }}>
        <button type="button" onClick={onCancel} style={{ ...buttonGhost, flex: 1 }}>
          Cancel
        </button>
        {/* Focused on open: Enter takes the password, Escape cancels. */}
        <button
          type="button"
          autoFocus
          onClick={() => onConfirm(gen.password)}
          disabled={!canConfirm}
          style={withDisabled({ ...buttonPrimary, flex: 1 }, !canConfirm)}
        >
          OK
        </button>
      </div>
    </>
  );
}
