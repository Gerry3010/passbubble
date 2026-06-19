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
import browser from 'webextension-polyfill';
import type { SessionInfo } from '@passbubble/shared-ts';
import { MessageType } from '../../shared/constants.js';
import { term, buttonPrimary, muted, withDisabled } from '../../shared/theme.js';

export function AccountSection() {
  const [session, setSession] = useState<SessionInfo | null>(null);
  const [busy, setBusy] = useState(false);
  const [loggedOut, setLoggedOut] = useState(false);

  useEffect(() => {
    browser.runtime
      .sendMessage({ type: MessageType.GET_SESSION, payload: {} })
      .then((resp) => setSession(resp as SessionInfo))
      .catch(() => setSession(null));
  }, []);

  async function logout() {
    setBusy(true);
    try {
      await browser.runtime.sendMessage({ type: MessageType.LOGOUT, payload: {} });
      setLoggedOut(true);
      setSession({ isLoggedIn: false, isUnlocked: false });
    } finally {
      setBusy(false);
    }
  }

  return (
    <section style={{ display: 'flex', flexDirection: 'column', gap: '12px', marginBottom: '32px' }}>
      <h2 style={{ fontSize: '18px', fontWeight: 700, color: term.green, fontFamily: term.font }}># account</h2>

      {loggedOut ? (
        <p style={muted}>Signed out. Open the extension popup to sign in again.</p>
      ) : session?.isLoggedIn ? (
        <>
          <p style={{ ...muted, fontSize: '14px' }}>
            Signed in as{' '}
            <span style={{ color: term.green }}>[{session.userName || session.userEmail || 'unknown'}]</span>
          </p>
          <button onClick={logout} disabled={busy} style={withDisabled({ ...buttonPrimary, alignSelf: 'flex-start', padding: '8px 16px' }, busy)}>
            {busy ? 'Signing out…' : 'Log out'}
          </button>
        </>
      ) : (
        <p style={muted}>Not signed in. Open the extension popup to sign in.</p>
      )}
    </section>
  );
}
