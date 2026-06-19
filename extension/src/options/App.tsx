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

import { ServerUrlForm } from './components/ServerUrlForm.js';
import { term } from '../shared/theme.js';

export function App() {
  return (
    <div style={{ padding: '8px 0' }}>
      <h1 style={{ fontSize: '24px', fontWeight: 700, marginBottom: '24px', color: term.green, fontFamily: term.font }}>
        <span style={{ color: term.muted }}>passbubble:~$</span> settings
      </h1>
      <ServerUrlForm />
      <section style={{ color: term.muted, fontSize: '13px' }}>
        <h2 style={{ fontSize: '16px', fontWeight: 700, color: term.green, marginBottom: '8px', fontFamily: term.font }}>
          # security
        </h2>
        <p>Your master password is never stored. The vault locks when the browser closes.</p>
      </section>
    </div>
  );
}
