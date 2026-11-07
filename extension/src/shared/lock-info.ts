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

/** Why the vault stopped being unlocked. */
export type LockReason =
  | 'manual'
  | 'idle'
  | 'pin-lockout'
  | 'signed-out'
  | 'browser-start'
  | 'extension-reload'
  | 'session-lost';

/** Where the idle timeout of an 'idle' lock came from. 'default' means neither
 * storage area had a value, so a user who chose "never" should not see it. */
export type AutoLockSource = 'sync' | 'local' | 'default';

export interface LockEvent {
  /** 'persist-failed' is no lock itself: it explains a later 'session-lost'. */
  reason: LockReason | 'persist-failed';
  at: number;
  idleMinutes?: number;
  settingSource?: AutoLockSource;
}

/** One line for the unlock screen and the options page. */
export function describeLock(event: LockEvent): string {
  switch (event.reason) {
    case 'manual':
      return 'you pressed lock';
    case 'idle':
      return event.settingSource === 'default'
        ? `idle for ${event.idleMinutes} min (default timeout, no saved setting found)`
        : `idle for ${event.idleMinutes} min`;
    case 'pin-lockout':
      return 'too many wrong PINs';
    case 'signed-out':
      return 'signed out';
    case 'browser-start':
      return 'the browser restarted';
    case 'extension-reload':
      return 'the extension was reloaded or updated';
    case 'session-lost':
      return 'the unlocked session disappeared (cause unknown)';
    case 'persist-failed':
      return 'could not keep the unlocked session across restarts of the background worker';
  }
}

export function formatLockTime(at: number): string {
  return new Date(at).toLocaleString(undefined, { dateStyle: 'short', timeStyle: 'short' });
}
