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

// Full edit form for an existing entry. Fields, labels and JSON keys follow the
// app's edit screen (flutter_app/lib/features/entries/add_edit_screen.dart). The
// form works out what the user changed and sends only that (see EntryEdit); the
// background applies it to the entry as it is at save time.

import { useEffect, useId, useState } from 'react';
import browser from 'webextension-polyfill';
import { parseTagInput, tagsOf, totpSecretOf } from '@passbubble/shared-ts';
import type { EntryData, EntryResponse } from '@passbubble/shared-ts';
import { MessageType } from '../../shared/constants.js';
import type { EntryEdit } from '../../shared/entry-edit.js';
import { SSO_PROVIDER_LABELS } from '../../shared/sso.js';
import { term, input, buttonPrimary, buttonGhost, link, muted, errorText, withDisabled } from '../../shared/theme.js';
import { clearEditDraft, getEditDraft, setEditDraft } from '../store/edit-draft.js';
import { handleLockedResponse } from '../store/session.js';
import { parseTotpInput, TagsInput } from './EntryFormParts.js';
import { GeneratorDialog } from './GeneratorDialog.js';

// ─── Fields per entry type ───────────────────────────────────────────────────

interface FieldSpec {
  key: string;
  label: string;
  secret?: boolean;
  /** Text area with this many rows. */
  rows?: number;
  /** Drop-down values and labels; an empty choice is always offered. */
  options?: Array<[value: string, label: string]>;
  generate?: boolean;
}

const NOTES: FieldSpec = { key: 'notes', label: 'Notes', rows: 3 };

const TYPE_FIELDS: Record<string, FieldSpec[]> = {
  password: [
    { key: 'username', label: 'Username' },
    { key: 'password', label: 'Password', secret: true, generate: true },
    { key: 'totp_secret', label: 'TOTP secret (base32 or otpauth:// URI)', secret: true },
    { key: 'sign_in_with', label: 'Sign in with (SSO)', options: Object.entries(SSO_PROVIDER_LABELS) },
    NOTES,
  ],
  totp: [
    { key: 'username', label: 'Account / Username' },
    { key: 'totp_secret', label: 'TOTP secret (base32 or otpauth:// URI)', secret: true },
    NOTES,
  ],
  note: [{ key: 'notes', label: 'Content', rows: 8 }],
  'api-key': [
    { key: 'username', label: 'Key ID / Client ID' },
    { key: 'password', label: 'API key / Secret', secret: true },
    NOTES,
  ],
  'ssh-key': [
    { key: 'username', label: 'User' },
    { key: 'password', label: 'Private key', secret: true, rows: 6 },
    { key: 'notes', label: 'Notes / Passphrase', rows: 3 },
  ],
  'credit-card': [
    { key: 'card_number', label: 'Card number', secret: true },
    { key: 'holder_name', label: 'Cardholder name' },
    { key: 'expiry_month', label: 'Expiry month (MM)' },
    { key: 'expiry_year', label: 'Expiry year (YYYY)' },
    { key: 'cvv', label: 'CVV', secret: true },
    NOTES,
  ],
  'bank-account': [
    { key: 'bank_name', label: 'Bank name' },
    { key: 'iban', label: 'IBAN' },
    { key: 'bic', label: 'BIC / SWIFT' },
    { key: 'account_number', label: 'Account number' },
    {
      key: 'account_type',
      label: 'Account type',
      options: [
        ['checking', 'Checking'],
        ['savings', 'Savings'],
      ],
    },
    NOTES,
  ],
  identity: [
    {
      key: 'title',
      label: 'Title',
      options: [
        ['Mr', 'Mr'],
        ['Ms', 'Ms'],
        ['Mrs', 'Mrs'],
        ['Dr', 'Dr'],
      ],
    },
    { key: 'first_name', label: 'First name' },
    { key: 'last_name', label: 'Last name' },
    { key: 'company', label: 'Company' },
    { key: 'email', label: 'Email' },
    { key: 'phone', label: 'Phone' },
    { key: 'street', label: 'Street' },
    { key: 'city', label: 'City' },
    { key: 'state', label: 'State' },
    { key: 'postal_code', label: 'Postal code' },
    { key: 'country', label: 'Country' },
    NOTES,
  ],
  license: [
    { key: 'product_name', label: 'Product name' },
    { key: 'license_key', label: 'License key', secret: true },
    { key: 'purchase_email', label: 'Purchase email' },
    { key: 'purchase_date', label: 'Purchase date (YYYY-MM-DD)' },
    { key: 'expires_at', label: 'Expires at (YYYY-MM-DD)' },
    NOTES,
  ],
};

// Like the app, an unknown type is edited as a login.
function fieldsFor(type: string): FieldSpec[] {
  return TYPE_FIELDS[type] ?? TYPE_FIELDS.password;
}

// Like the app, these types have no URL field.
const TYPES_WITHOUT_URL = new Set(['identity', 'bank-account']);

// ─── Custom fields ───────────────────────────────────────────────────────────

const CUSTOM_TYPES: Array<[value: string, label: string]> = [
  ['text', 'Text'],
  ['password', 'Password'],
  ['ssh', 'SSH key'],
  ['totp', 'TOTP'],
];
const SECRET_CUSTOM_TYPES = new Set(['password', 'ssh', 'totp']);
const MULTILINE_CUSTOM_TYPES = new Set(['ssh', 'note']);

interface StoredCustomField {
  label?: unknown;
  value?: unknown;
  type?: unknown;
  filename?: unknown;
}

interface CustomRow {
  rowId: number;
  /** The stored list item as read (null for a row added here, unless locked).
   * Keys of it this form does not manage are written back with the row. */
  source: unknown;
  label: string;
  value: string;
  type: string;
  /** File fields and anything else this form cannot edit: written back exactly
   * as read, at their place in the list. */
  locked: boolean;
}

let lastRowId = 0;

function rowFrom(source: unknown): CustomRow {
  const isObject = typeof source === 'object' && source !== null;
  const cf: StoredCustomField = isObject ? (source as StoredCustomField) : {};
  return {
    rowId: ++lastRowId,
    source,
    label: typeof cf.label === 'string' ? cf.label : '',
    value: typeof cf.value === 'string' ? cf.value : '',
    type: typeof cf.type === 'string' && cf.type ? cf.type : 'text',
    locked: !isObject || cf.type === 'file',
  };
}

function newRow(): CustomRow {
  return { rowId: ++lastRowId, source: null, label: '', value: '', type: 'text', locked: false };
}

/** Moves a row past its nearest editable neighbour, so locked rows keep their
 * index. Returns the same list when there is nowhere to go. */
function moveRow(rows: CustomRow[], rowId: number, direction: -1 | 1): CustomRow[] {
  const from = rows.findIndex((r) => r.rowId === rowId);
  const to = neighbourIndex(rows, from, direction);
  if (to < 0) return rows;
  const next = [...rows];
  [next[from], next[to]] = [next[to], next[from]];
  return next;
}

function neighbourIndex(rows: CustomRow[], from: number, direction: -1 | 1): number {
  for (let i = from + direction; i >= 0 && i < rows.length; i += direction) {
    if (!rows[i].locked) return i;
  }
  return -1;
}

// ─── Form model ──────────────────────────────────────────────────────────────

/** The entry as loaded; what the form compares against. */
interface Baseline {
  name: string;
  url: string;
  fields: Record<string, string>;
  tags: string[];
  customFields: unknown[];
}

interface FormState {
  name: string;
  url: string;
  fields: Record<string, string>;
  tagsText: string;
  rows: CustomRow[];
}

interface FormModel {
  baseline: Baseline;
  state: FormState;
  /** The user has changed something, so the state is worth keeping as a draft. */
  touched: boolean;
}

function modelFromEntry(entry: EntryResponse, data: EntryData): FormModel {
  const raw = data as Record<string, unknown>;
  const fields: Record<string, string> = {};
  for (const { key } of fieldsFor(entry.type)) {
    // A stray copy of the password in totp_secret reads as no secret at all.
    const value = key === 'totp_secret' ? totpSecretOf(data) : raw[key];
    fields[key] = typeof value === 'string' ? value : '';
  }
  const tags = tagsOf(data);
  const customFields: unknown[] = Array.isArray(raw.custom_fields) ? raw.custom_fields : [];
  const baseline = { name: entry.name, url: entry.url ?? '', fields, tags, customFields };
  return {
    baseline,
    state: {
      name: entry.name,
      url: baseline.url,
      fields: { ...fields },
      tagsText: tags.length > 0 ? `${tags.join(', ')}, ` : '',
      rows: customFields.map(rowFrom),
    },
    touched: false,
  };
}

// A password or private key is stored as typed: outer whitespace can be part of
// it (an SSH key needs its final newline). Everything else is trimmed like in
// the app. Only blank means removal.
function cleaned(value: string, verbatim: boolean): string {
  const trimmed = value.trim();
  return verbatim && trimmed ? value : trimmed;
}

function changedFields(type: string, baseline: Baseline, state: FormState): NonNullable<EntryEdit['fields']> | string {
  const out: NonNullable<EntryEdit['fields']> = {};
  for (const { key } of fieldsFor(type)) {
    const value = state.fields[key] ?? '';
    if (value === (baseline.fields[key] ?? '')) continue;
    if (key === 'totp_secret') {
      const parsed = parseTotpInput(value);
      if (parsed === null) return '2FA secret is not a valid base32 string or otpauth:// URI';
      out.totp_secret = parsed.totp_secret ?? '';
      // A pasted URI defines the code completely, also by what it leaves out.
      if ('issuer' in parsed) {
        out.issuer = parsed.issuer ?? null;
        out.period = parsed.period ?? null;
        out.digits = parsed.digits ?? null;
        out.algorithm = parsed.algorithm ?? null;
      }
      continue;
    }
    out[key] = cleaned(value, key === 'password');
  }
  return out;
}

function customFieldList(rows: CustomRow[]): unknown[] | string {
  const list: unknown[] = [];
  for (const row of rows) {
    if (row.locked) {
      list.push(row.source);
      continue;
    }
    const source = row.source as StoredCustomField | null;
    const unchanged =
      source !== null &&
      (source.label ?? '') === row.label &&
      (source.value ?? '') === row.value &&
      ((source.type as string | undefined) || 'text') === row.type;
    if (unchanged) {
      list.push(source);
      continue;
    }
    const label = row.label.trim();
    const value = cleaned(row.value, SECRET_CUSTOM_TYPES.has(row.type));
    if (!label) {
      // Never drop a value without a word: only an empty row goes away.
      if (value) return 'A custom field with a value needs a label';
      continue;
    }
    list.push({ ...source, label, value, type: row.type });
  }
  return list;
}

/** What to send on save, or why it cannot be saved. `changed` is false when the
 * entry would stay exactly as it is, which writes nothing (and adds no version
 * to the entry's history). */
function buildEdit(type: string, { baseline, state }: FormModel): { edit: EntryEdit; changed: boolean } | { error: string } {
  const name = state.name.trim();
  if (!name) return { error: 'Name is required' };
  const url = state.url.trim();

  const fields = changedFields(type, baseline, state);
  if (typeof fields === 'string') return { error: fields };

  const { tags, error: tagError } = parseTagInput(state.tagsText);
  if (tagError) return { error: tagError };
  const tagsChanged = JSON.stringify(tags) !== JSON.stringify(baseline.tags);

  const customFields = customFieldList(state.rows);
  if (typeof customFields === 'string') return { error: customFields };
  const customChanged =
    customFields.length !== baseline.customFields.length || customFields.some((cf, i) => cf !== baseline.customFields[i]);

  return {
    edit: {
      ...(name !== baseline.name ? { name } : {}),
      // '' clears the URL; left out when unchanged, which keeps it.
      ...(url !== baseline.url ? { url } : {}),
      fields,
      ...(tagsChanged ? { tags } : {}),
      ...(customChanged ? { customFields } : {}),
    },
    changed:
      name !== baseline.name ||
      url !== baseline.url ||
      Object.keys(fields).length > 0 ||
      tagsChanged ||
      customChanged,
  };
}

// ─── Form ────────────────────────────────────────────────────────────────────

type GeneratorTarget = { kind: 'field'; key: string } | { kind: 'row'; rowId: number };

export function EditEntryForm({
  entry,
  vaultTags = [],
  onCancel,
  onSaved,
}: {
  entry: EntryResponse;
  /** Tags already used in the vault, offered as completions. */
  vaultTags?: string[];
  onCancel: () => void;
  /** Saved (or nothing to save); hands back the metadata as it now is. */
  onSaved: (meta: { name: string; url?: string }) => void;
}) {
  const [model, setModel] = useState<FormModel | null>(() => {
    const draft = getEditDraft();
    return draft?.entry.id === entry.id ? (draft.model as FormModel) : null;
  });
  const [restored] = useState(model !== null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [generatorFor, setGeneratorFor] = useState<GeneratorTarget | null>(null);

  useEffect(() => {
    if (model) return;
    let alive = true;
    browser.runtime
      .sendMessage({ type: MessageType.GET_ENTRY, payload: { id: entry.id } })
      .then((resp) => {
        if (!alive) return;
        const r = resp as { entry?: EntryResponse; data?: EntryData; locked?: boolean };
        if (handleLockedResponse(r) || !r.data) {
          setLoadError('Vault is locked');
          return;
        }
        setModel(modelFromEntry(r.entry ?? entry, r.data));
      })
      .catch((e) => {
        if (alive) setLoadError(`Failed to load entry: ${e instanceof Error ? e.message : String(e)}`);
      });
    return () => {
      alive = false;
    };
  }, [entry.id]);

  // Whatever removes this form while it holds changes (the vault locking, a tab
  // switch), the input is still there when the vault panel comes back.
  useEffect(() => {
    if (model?.touched) setEditDraft({ entry, model });
  }, [model, entry]);

  function cancel() {
    clearEditDraft();
    onCancel();
  }

  if (!model) {
    return (
      <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
        <button type="button" onClick={cancel} style={{ ...link, alignSelf: 'flex-start' }}>
          ‹ Back
        </button>
        {loadError ? <p style={errorText}>{loadError}</p> : <p style={muted}>Loading…</p>}
      </div>
    );
  }

  const { state } = model;

  function update(patch: Partial<FormState>) {
    setModel((m) => m && { ...m, state: { ...m.state, ...patch }, touched: true });
  }

  function setField(key: string, value: string) {
    setModel((m) => m && { ...m, state: { ...m.state, fields: { ...m.state.fields, [key]: value } }, touched: true });
  }

  function setRows(change: (rows: CustomRow[]) => CustomRow[]) {
    setModel((m) => m && { ...m, state: { ...m.state, rows: change(m.state.rows) }, touched: true });
  }

  function patchRow(rowId: number, patch: Partial<CustomRow>) {
    setRows((rows) => rows.map((r) => (r.rowId === rowId ? { ...r, ...patch } : r)));
  }

  function takeGenerated(password: string) {
    if (generatorFor?.kind === 'field') setField(generatorFor.key, password);
    if (generatorFor?.kind === 'row') patchRow(generatorFor.rowId, { value: password });
    setGeneratorFor(null);
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    if (!model) return;
    const result = buildEdit(entry.type, model);
    if ('error' in result) {
      setSaveError(result.error);
      return;
    }
    const { edit, changed } = result;
    const savedMeta = { name: state.name.trim(), url: state.url.trim() };
    if (!changed) {
      clearEditDraft();
      onSaved(savedMeta);
      return;
    }
    setBusy(true);
    setSaveError(null);
    try {
      const resp = (await browser.runtime.sendMessage({
        type: MessageType.EDIT_ENTRY,
        payload: { entryId: entry.id, ...edit },
      })) as { ok?: boolean; error?: string; locked?: boolean };
      if (handleLockedResponse(resp)) {
        setSaveError('Vault is locked — nothing was saved. Your changes come back after unlocking.');
        return;
      }
      if (!resp?.ok) {
        setSaveError(resp?.error ?? 'Could not save the entry.');
        return;
      }
      clearEditDraft();
      onSaved(savedMeta);
    } catch (err) {
      setSaveError(`Could not save the entry: ${err instanceof Error ? err.message : String(err)}`);
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <form onSubmit={submit} style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
        <button type="button" onClick={cancel} style={{ ...link, alignSelf: 'flex-start' }}>
          ‹ Back
        </button>
        <h3 style={{ margin: 0, fontSize: '15px', color: term.green }}># edit entry</h3>
        <span style={muted}>type: {entry.type}</span>
        {restored && <p style={{ ...muted, color: term.amber }}>Your unsaved changes were restored. Nothing is saved yet.</p>}

        <FieldInput label="Name" value={state.name} onChange={(name) => update({ name })} />
        {!TYPES_WITHOUT_URL.has(entry.type) && (
          <FieldInput label="URL" value={state.url} onChange={(url) => update({ url })} />
        )}
        {fieldsFor(entry.type).map((spec) => (
          <FieldInput
            key={spec.key}
            label={spec.label}
            value={state.fields[spec.key] ?? ''}
            secret={spec.secret}
            rows={spec.rows}
            options={spec.options}
            onChange={(value) => setField(spec.key, value)}
            onGenerate={spec.generate ? () => setGeneratorFor({ kind: 'field', key: spec.key }) : undefined}
          />
        ))}

        <div style={{ display: 'flex', flexDirection: 'column', gap: '2px' }}>
          <span style={{ fontSize: '10px', color: term.muted }}>Tags</span>
          <div style={{ display: 'flex', flexDirection: 'column', gap: '4px' }}>
            <TagsInput value={state.tagsText} onChange={(tagsText) => update({ tagsText })} vaultTags={vaultTags} />
          </div>
        </div>

        <span style={{ fontSize: '11px', color: term.green, marginTop: '4px' }}># custom fields</span>
        {state.rows.map((row, index) =>
          row.locked ? (
            <LockedRow key={row.rowId} row={row} />
          ) : (
            <CustomFieldRow
              key={row.rowId}
              row={row}
              number={index + 1}
              canMoveUp={neighbourIndex(state.rows, index, -1) >= 0}
              canMoveDown={neighbourIndex(state.rows, index, 1) >= 0}
              onChange={(patch) => patchRow(row.rowId, patch)}
              onMove={(direction) => setRows((rows) => moveRow(rows, row.rowId, direction))}
              onRemove={() => setRows((rows) => rows.filter((r) => r.rowId !== row.rowId))}
              onGenerate={() => setGeneratorFor({ kind: 'row', rowId: row.rowId })}
            />
          ),
        )}
        <button
          type="button"
          onClick={() => setRows((rows) => [...rows, newRow()])}
          style={{ ...buttonGhost, padding: '4px 8px', fontSize: '12px' }}
        >
          + Add field
        </button>

        {saveError && (
          <p role="alert" style={errorText}>
            {saveError}
          </p>
        )}
        <div style={{ display: 'flex', gap: '6px' }}>
          <button type="button" onClick={cancel} style={{ ...buttonGhost, flex: 1 }}>
            Cancel
          </button>
          <button type="submit" disabled={busy} style={withDisabled({ ...buttonPrimary, flex: 1 }, busy)}>
            {busy ? 'Saving…' : 'Save changes'}
          </button>
        </div>
      </form>
      {/* Outside the form, so Enter in the dialog's inputs cannot submit the entry. */}
      {generatorFor && <GeneratorDialog onCancel={() => setGeneratorFor(null)} onConfirm={takeGenerated} />}
    </>
  );
}

// ─── Inputs ──────────────────────────────────────────────────────────────────

const smallLabel: React.CSSProperties = { fontSize: '10px', color: term.muted };
const fieldHeader: React.CSSProperties = { display: 'flex', justifyContent: 'space-between', gap: '8px' };

function FieldInput({
  label,
  value,
  onChange,
  secret = false,
  rows,
  options,
  onGenerate,
}: {
  label: string;
  value: string;
  onChange: (next: string) => void;
  secret?: boolean;
  rows?: number;
  options?: Array<[value: string, label: string]>;
  onGenerate?: () => void;
}) {
  const id = useId();
  const [revealed, setRevealed] = useState(false);
  const hidden = secret && !revealed;

  let control: React.ReactNode;
  if (options) {
    // A stored value outside the list stays selectable, and so unchanged.
    const known = value === '' || options.some(([v]) => v === value);
    control = (
      <select id={id} style={input} value={value} onChange={(e) => onChange(e.target.value)}>
        <option value="">—</option>
        {options.map(([v, text]) => (
          <option key={v} value={v}>
            {text}
          </option>
        ))}
        {!known && <option value={value}>{value}</option>}
      </select>
    );
  } else if (rows && hidden && value) {
    // A masked input would drop the line breaks of a multi-line secret on the
    // first keystroke, so while hidden it cannot be typed into.
    control = (
      <div id={id} role="textbox" aria-readonly="true" aria-label={label} style={{ ...input, color: term.muted }}>
        •••••••• (Show to edit)
      </div>
    );
  } else if (rows) {
    control = (
      <textarea
        id={id}
        style={{ ...input, minHeight: `${rows * 16 + 16}px`, resize: 'vertical' }}
        value={value}
        onChange={(e) => {
          // Typed into an empty secret: keep showing what is being typed.
          if (secret) setRevealed(true);
          onChange(e.target.value);
        }}
      />
    );
  } else {
    control = (
      <input
        id={id}
        style={input}
        type={hidden ? 'password' : 'text'}
        autoComplete={secret ? 'new-password' : 'off'}
        value={value}
        onChange={(e) => onChange(e.target.value)}
      />
    );
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '2px' }}>
      <div style={fieldHeader}>
        <label htmlFor={id} style={smallLabel}>
          {label}
        </label>
        <span style={{ display: 'flex', gap: '8px' }}>
          {secret && (
            <button
              type="button"
              aria-label={`${revealed ? 'Hide' : 'Show'} ${label}`}
              onClick={() => setRevealed((v) => !v)}
              style={{ ...link, fontSize: '10px' }}
            >
              {revealed ? 'Hide' : 'Show'}
            </button>
          )}
          {onGenerate && (
            <button
              type="button"
              aria-label={`Generate ${label}`}
              onClick={onGenerate}
              style={{ ...link, fontSize: '10px' }}
            >
              Generate
            </button>
          )}
        </span>
      </div>
      {control}
    </div>
  );
}

const rowBox: React.CSSProperties = {
  display: 'flex',
  flexDirection: 'column',
  gap: '4px',
  padding: '6px',
  border: `1px solid ${term.border}`,
  borderRadius: '4px',
  background: term.surface,
};

const rowButton: React.CSSProperties = { ...buttonGhost, padding: '2px 6px', fontSize: '11px' };

function CustomFieldRow({
  row,
  number,
  canMoveUp,
  canMoveDown,
  onChange,
  onMove,
  onRemove,
  onGenerate,
}: {
  row: CustomRow;
  number: number;
  canMoveUp: boolean;
  canMoveDown: boolean;
  onChange: (patch: Partial<CustomRow>) => void;
  onMove: (direction: -1 | 1) => void;
  onRemove: () => void;
  onGenerate: () => void;
}) {
  const name = `custom field ${number}`;
  // A type outside this form's choices (the app has more) stays as it is.
  const typeKnown = CUSTOM_TYPES.some(([v]) => v === row.type);
  return (
    <div style={rowBox} data-testid="custom-field">
      <div style={{ display: 'flex', gap: '4px' }}>
        <button
          type="button"
          aria-label={`Move ${name} up`}
          disabled={!canMoveUp}
          onClick={() => onMove(-1)}
          style={withDisabled(rowButton, !canMoveUp)}
        >
          ↑
        </button>
        <button
          type="button"
          aria-label={`Move ${name} down`}
          disabled={!canMoveDown}
          onClick={() => onMove(1)}
          style={withDisabled(rowButton, !canMoveDown)}
        >
          ↓
        </button>
        <select
          aria-label={`Type of ${name}`}
          style={{ ...input, padding: '2px 4px', fontSize: '11px', flex: 1, minWidth: 0 }}
          value={row.type}
          onChange={(e) => onChange({ type: e.target.value })}
        >
          {CUSTOM_TYPES.map(([value, text]) => (
            <option key={value} value={value}>
              {text}
            </option>
          ))}
          {!typeKnown && <option value={row.type}>{row.type}</option>}
        </select>
        <button type="button" aria-label={`Remove ${name}`} onClick={onRemove} style={{ ...rowButton, color: term.red }}>
          ×
        </button>
      </div>
      <FieldInput label={`Label of ${name}`} value={row.label} onChange={(label) => onChange({ label })} />
      {/* Remount on a type change: the field starts masked again. */}
      <FieldInput
        key={row.type}
        label={`Value of ${name}`}
        value={row.value}
        secret={SECRET_CUSTOM_TYPES.has(row.type)}
        rows={MULTILINE_CUSTOM_TYPES.has(row.type) ? 4 : undefined}
        onChange={(value) => onChange({ value })}
        onGenerate={row.type === 'password' ? onGenerate : undefined}
      />
    </div>
  );
}

function LockedRow({ row }: { row: CustomRow }) {
  const cf = (row.source ?? {}) as StoredCustomField;
  const isFile = row.type === 'file';
  return (
    <div style={rowBox} data-testid="custom-field">
      <span style={{ fontSize: '12px', color: term.green }}>{row.label || (isFile ? 'File' : 'Unreadable field')}</span>
      <span style={{ ...muted, fontSize: '11px' }}>
        {isFile ? `file: ${typeof cf.filename === 'string' ? cf.filename : 'attachment'} (read-only)` : 'kept as it is'}
      </span>
    </div>
  );
}
