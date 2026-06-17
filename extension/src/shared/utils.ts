// Standard base64 encoding/decoding (with + / = padding, NOT url-safe).
// Matches Go's base64.StdEncoding used throughout the backend and CLI.

export function b64Enc(bytes: Uint8Array): string {
  return btoa(String.fromCharCode(...bytes));
}

export function b64Dec(s: string): Uint8Array {
  const binary = atob(s);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
  return bytes;
}

// Normalise a URL for host-based matching: strip www. prefix and path.
export function normaliseHost(url: string): string {
  try {
    const u = new URL(url.startsWith('http') ? url : `https://${url}`);
    return u.hostname.replace(/^www\./, '');
  } catch {
    return '';
  }
}

// Returns true if pageHost matches entryHost (including subdomain of entryHost).
export function hostMatches(pageHost: string, entryHost: string): boolean {
  if (!pageHost || !entryHost) return false;
  return pageHost === entryHost || pageHost.endsWith('.' + entryHost);
}
