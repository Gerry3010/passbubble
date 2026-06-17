import { vi, beforeEach } from 'vitest';

// Make all HTML elements "visible" in jsdom (offsetParent is always null in jsdom)
Object.defineProperty(HTMLElement.prototype, 'offsetParent', {
  get() {
    return document.body;
  },
});

// Mock the Clipboard API (not available in jsdom)
Object.defineProperty(navigator, 'clipboard', {
  value: {
    writeText: vi.fn().mockResolvedValue(undefined),
    readText: vi.fn().mockResolvedValue(''),
  },
  writable: true,
});

beforeEach(() => {
  vi.clearAllMocks();
  // Clean up any DOM changes between tests
  document.body.innerHTML = '';
});
