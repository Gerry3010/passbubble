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

import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('webextension-polyfill', () => ({
  default: {
    runtime: { sendMessage: vi.fn() },
  },
}));

import browser from 'webextension-polyfill';
import { initSaveDetector, recoverPendingSave } from '../save-detector.js';
import { MessageType } from '../../shared/constants.js';

const mockSendMessage = browser.runtime.sendMessage as ReturnType<typeof vi.fn>;

// The save bar is injected as a fixed-position host <div> on document.body.
function barShown(): boolean {
  return Array.from(document.body.children).some(
    (el) => el instanceof HTMLElement && el.style.position === 'fixed',
  );
}

function buildForm(username = 'alice', password = 's3cr3t'): HTMLFormElement {
  const form = document.createElement('form');
  const u = document.createElement('input');
  u.type = 'email';
  u.value = username;
  const p = document.createElement('input');
  p.type = 'password';
  p.value = password;
  form.appendChild(u);
  form.appendChild(p);
  document.body.appendChild(form);
  return form;
}

async function submitForm(form: HTMLFormElement): Promise<void> {
  form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
  // Flush microtasks so the async listener runs to completion
  await new Promise((r) => setTimeout(r, 0));
}

describe('initSaveDetector', () => {
  beforeEach(() => {
    document.body.innerHTML = '';
    // The background's OFFER_SAVE handler decides whether to offer (locked vault,
    // blocklisted/dismissed host, or unchanged credentials) and returns the
    // existing matches. Content scripts must not read storage.session, so the
    // detector always asks and renders based on the answer. Default: offer.
    mockSendMessage.mockResolvedValue({ ok: true });
    initSaveDetector();
  });

  it('sends OFFER_SAVE with username and password on form submit', async () => {
    const form = buildForm('alice@example.com', 'hunter2');
    await submitForm(form);

    expect(mockSendMessage).toHaveBeenCalledWith(
      expect.objectContaining({
        type: MessageType.OFFER_SAVE,
        payload: expect.objectContaining({
          username: 'alice@example.com',
          password: 'hunter2',
        }),
      }),
    );
  });

  it('includes the current page URL in the OFFER_SAVE payload', async () => {
    const form = buildForm();
    await submitForm(form);

    expect(mockSendMessage).toHaveBeenCalledWith(
      expect.objectContaining({
        type: MessageType.OFFER_SAVE,
        payload: expect.objectContaining({ url: 'https://example.com/login' }),
      }),
    );
  });

  it('does NOT call preventDefault on the submit event', async () => {
    const form = buildForm();
    const spy = vi.spyOn(Event.prototype, 'preventDefault');
    await submitForm(form);
    expect(spy).not.toHaveBeenCalled();
  });

  it('delegates dismissed-host filtering to the background (no direct storage read)', async () => {
    // The webextension mock exposes NO storage API, so any direct storage access
    // from the content script would throw — reaching OFFER_SAVE proves it doesn't.
    const form = buildForm();
    await submitForm(form);

    expect(mockSendMessage).toHaveBeenCalledWith(
      expect.objectContaining({ type: MessageType.OFFER_SAVE }),
    );
  });

  it('still sends OFFER_SAVE on a known site (the background decides new vs update)', async () => {
    // Even when entries exist, the detector no longer short-circuits — it asks the
    // background, which returns candidates / an update suggestion.
    mockSendMessage.mockResolvedValue({
      ok: true,
      candidates: [{ id: '1', username: 'alice' }],
      suggestUpdateId: '1',
    });
    const form = buildForm();
    await submitForm(form);

    expect(mockSendMessage).toHaveBeenCalledWith(
      expect.objectContaining({ type: MessageType.OFFER_SAVE }),
    );
    expect(barShown()).toBe(true);
  });

  it('shows no save bar when the background declines the offer', async () => {
    mockSendMessage.mockResolvedValue({ ok: false, unchanged: true });
    const form = buildForm();
    await submitForm(form);

    expect(barShown()).toBe(false);
  });

  it('picks the filled username field over an empty unrelated one', async () => {
    const form = document.createElement('form');
    const search = document.createElement('input');
    search.type = 'text';
    search.name = 'search';
    search.value = ''; // empty, unrelated
    const email = document.createElement('input');
    email.type = 'email';
    email.name = 'email';
    email.value = 'me@example.com';
    const pw = document.createElement('input');
    pw.type = 'password';
    pw.value = 'hunter2';
    form.append(search, email, pw);
    document.body.appendChild(form);

    await submitForm(form);

    expect(mockSendMessage).toHaveBeenCalledWith(
      expect.objectContaining({
        type: MessageType.OFFER_SAVE,
        payload: expect.objectContaining({ username: 'me@example.com' }),
      }),
    );
  });

  it('sends an empty username (and still offers) when none is detected', async () => {
    const form = document.createElement('form');
    const pw = document.createElement('input');
    pw.type = 'password';
    pw.value = 'hunter2';
    form.appendChild(pw);
    document.body.appendChild(form);

    await submitForm(form);

    expect(mockSendMessage).toHaveBeenCalledWith(
      expect.objectContaining({
        type: MessageType.OFFER_SAVE,
        payload: expect.objectContaining({ username: '' }),
      }),
    );
  });

  it('skips when form has no password field value', async () => {
    const form = document.createElement('form');
    const pw = document.createElement('input');
    pw.type = 'password';
    pw.value = ''; // empty
    form.appendChild(pw);
    document.body.appendChild(form);

    await submitForm(form);
    expect(mockSendMessage).not.toHaveBeenCalledWith(
      expect.objectContaining({ type: MessageType.OFFER_SAVE }),
    );
  });
});

describe('recoverPendingSave', () => {
  beforeEach(() => {
    document.body.innerHTML = '';
    mockSendMessage.mockReset();
  });

  it('re-shows the save bar when a pending save matches the current page', async () => {
    // jsdom serves the page at https://example.com/login (see vitest config).
    mockSendMessage.mockResolvedValue({
      url: 'https://example.com/account',
      username: 'alice',
      candidates: [{ id: '1', username: 'alice' }],
      suggestUpdateId: '1',
    });

    await recoverPendingSave();

    expect(mockSendMessage).toHaveBeenCalledWith(
      expect.objectContaining({ type: MessageType.GET_PENDING_SAVE }),
    );
    expect(barShown()).toBe(true);
  });

  it('does NOT show the bar for a pending save from a different domain', async () => {
    mockSendMessage.mockResolvedValue({ url: 'https://other.test/login', username: 'alice' });

    await recoverPendingSave();

    expect(barShown()).toBe(false);
  });

  it('does nothing when there is no pending save', async () => {
    mockSendMessage.mockResolvedValue(null);

    await recoverPendingSave();

    expect(barShown()).toBe(false);
  });
});
