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

export function App() {
  return (
    <div style={{ padding: '32px 0' }}>
      <h1 style={{ fontSize: '24px', fontWeight: 700, marginBottom: '24px', color: '#2d3748' }}>
        🔐 Passbubble Settings
      </h1>
      <ServerUrlForm />
      <section style={{ color: '#718096', fontSize: '13px' }}>
        <h2 style={{ fontSize: '16px', fontWeight: 600, color: '#4a5568', marginBottom: '8px' }}>
          Security
        </h2>
        <p>Your master password is never stored. The vault locks when the browser closes.</p>
      </section>
    </div>
  );
}
