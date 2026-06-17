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
