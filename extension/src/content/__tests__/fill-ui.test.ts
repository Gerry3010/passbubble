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
    runtime: {
      id: 'test',
      getURL: (p: string) => `chrome-extension://test/${p}`,
      sendMessage: vi.fn(),
    },
  },
}));

import { injectFillIframe, removeFillIframe } from '../fill-ui.js';

const noHandlers = { onFillMatch: () => {}, onUseGenerated: () => {}, onDismiss: () => {} };

function anchorAt(top: number, bottom: number, left: number): HTMLInputElement {
  const anchor = document.createElement('input');
  anchor.type = 'password';
  document.body.appendChild(anchor);
  anchor.getBoundingClientRect = () =>
    ({ top, bottom, left, right: left + 200, width: 200, height: bottom - top, x: left, y: top, toJSON: () => ({}) }) as DOMRect;
  return anchor;
}

function iframeEl(): HTMLIFrameElement {
  const el = document.querySelector('iframe');
  if (!el) throw new Error('no fill iframe injected');
  return el;
}

// jsdom does not implement elementsFromPoint — install a mock per test.
function mockElementsFromPoint(impl: (x: number, y: number) => Element[]): void {
  (document as unknown as { elementsFromPoint: (x: number, y: number) => Element[] }).elementsFromPoint =
    vi.fn(impl);
}

describe('fill-iframe positioning', () => {
  beforeEach(() => {
    removeFillIframe();
    document.body.innerHTML = '';
  });

  it('places the iframe below the anchor when nothing interactive is underneath', () => {
    mockElementsFromPoint(() => []);
    const anchor = anchorAt(100, 130, 50);

    injectFillIframe(anchor, { matches: [] }, noHandlers);

    expect(iframeEl().style.top).toBe('134px'); // bottom + 4
    expect(iframeEl().style.left).toBe('50px');
  });

  it('flips above the anchor when a button would be covered below', () => {
    const button = document.createElement('button');
    document.body.appendChild(button);
    mockElementsFromPoint(() => [button]);
    const anchor = anchorAt(100, 130, 50);

    injectFillIframe(anchor, { matches: [] }, noHandlers);

    // height falls back to the initial style height (56) → 100 - 56 - 4
    expect(iframeEl().style.top).toBe('40px');
  });

  it('stays below when flipping above would leave the viewport', () => {
    const button = document.createElement('button');
    document.body.appendChild(button);
    mockElementsFromPoint(() => [button]);
    const anchor = anchorAt(10, 40, 50); // too close to the top to flip

    injectFillIframe(anchor, { matches: [] }, noHandlers);

    expect(iframeEl().style.top).toBe('44px');
  });

  it('ignores the anchor field itself in the collision probe', () => {
    const anchor = anchorAt(100, 130, 50);
    mockElementsFromPoint(() => [anchor]);

    injectFillIframe(anchor, { matches: [] }, noHandlers);

    expect(iframeEl().style.top).toBe('134px');
  });

  it('re-positions when the page scrolls', () => {
    mockElementsFromPoint(() => []);
    const anchor = anchorAt(100, 130, 50);
    injectFillIframe(anchor, { matches: [] }, noHandlers);

    anchor.getBoundingClientRect = () =>
      ({ top: 60, bottom: 90, left: 50, right: 250, width: 200, height: 30, x: 50, y: 60, toJSON: () => ({}) }) as DOMRect;
    window.dispatchEvent(new Event('scroll'));

    expect(iframeEl().style.top).toBe('94px');
  });

  it('stops re-positioning after removal', () => {
    mockElementsFromPoint(() => []);
    const anchor = anchorAt(100, 130, 50);
    injectFillIframe(anchor, { matches: [] }, noHandlers);
    removeFillIframe();

    expect(() => window.dispatchEvent(new Event('scroll'))).not.toThrow();
    expect(document.querySelector('iframe')).toBeNull();
  });
});
