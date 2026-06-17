import type { EntryResponse } from '@passbubble/shared-ts';
import { hostMatches, normaliseHost } from '../shared/utils.js';

export function matchEntriesForUrl(url: string, entries: EntryResponse[]): EntryResponse[] {
  const pageHost = normaliseHost(url);
  if (!pageHost) return [];
  return entries.filter((e) => {
    if (!e.url) return false;
    const entryHost = normaliseHost(e.url);
    return hostMatches(pageHost, entryHost);
  });
}
