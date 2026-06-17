import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('webextension-polyfill', () => ({
  default: {
    runtime: { sendMessage: vi.fn() },
    storage: {
      session: { get: vi.fn() },
    },
  },
}));

import browser from 'webextension-polyfill';
import { initSaveDetector } from '../save-detector.js';
import { MessageType, STORAGE_KEYS } from '../../shared/constants.js';

const mockSendMessage = browser.runtime.sendMessage as ReturnType<typeof vi.fn>;
const mockSessionGet = (browser.storage.session as { get: ReturnType<typeof vi.fn> }).get;

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
    // By default: no dismissed hosts, no existing matches for current URL
    mockSessionGet.mockResolvedValue({ [STORAGE_KEYS.DISMISSED_SAVE_HOSTS]: [] });
    mockSendMessage.mockImplementation(async (msg: { type: string }) => {
      if (msg.type === MessageType.GET_MATCHES_FOR_URL) return []; // no existing entry
      return { ok: true };
    });
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

  it('skips OFFER_SAVE when the host is in the dismissed list', async () => {
    mockSessionGet.mockResolvedValue({
      [STORAGE_KEYS.DISMISSED_SAVE_HOSTS]: ['example.com'],
    });
    const form = buildForm();
    await submitForm(form);

    const offerCalls = mockSendMessage.mock.calls.filter(
      ([msg]: [{ type: string }]) => msg.type === MessageType.OFFER_SAVE,
    );
    expect(offerCalls).toHaveLength(0);
  });

  it('skips OFFER_SAVE when an existing entry already matches the URL', async () => {
    mockSendMessage.mockImplementation(async (msg: { type: string }) => {
      if (msg.type === MessageType.GET_MATCHES_FOR_URL)
        return [{ id: '1', name: 'Existing', url: 'https://example.com' }];
      return { ok: true };
    });
    const form = buildForm();
    await submitForm(form);

    const offerCalls = mockSendMessage.mock.calls.filter(
      ([msg]: [{ type: string }]) => msg.type === MessageType.OFFER_SAVE,
    );
    expect(offerCalls).toHaveLength(0);
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
