import { describe, expect, it } from 'vitest';
import { readFile } from 'fs/promises';
import { join, dirname } from 'path';
import { fileURLToPath } from 'url';
import { unlock, decryptEntry, encryptEntry, b64Dec, b64Enc } from '../vault.js';
import { generateX25519 } from '../../crypto/x25519.js';
import { generateMLKEM768 } from '../../crypto/mlkem.js';
import { deriveKey } from '../../crypto/argon2.js';
import { aesGcmEncrypt } from '../../crypto/aes-gcm.js';

const __dirname = dirname(fileURLToPath(import.meta.url));

describe('vault', () => {
  it('unlock derives private keys and returns them in memory', async () => {
    // Use reduced params for speed
    const password = 'test-master-pw';
    const { priv: privX, pub: pubX } = generateX25519();
    const { priv: privM, pub: pubM } = await generateMLKEM768();

    const salt = crypto.getRandomValues(new Uint8Array(32));
    const params = { salt, time: 1, memory: 8192 };
    const masterKey = await deriveKey(password, params);

    const encPrivX = await aesGcmEncrypt(masterKey, privX);
    const encPrivM = await aesGcmEncrypt(masterKey, privM);

    const { privX25519, privMLKEM } = await unlock(
      password,
      params,
      b64Enc(encPrivX),
      b64Enc(encPrivM),
    );

    expect(privX25519).toEqual(privX);
    expect(privMLKEM).toEqual(privM);
  }, 30_000);

  it('unlock with wrong password throws', async () => {
    const { priv: privX } = generateX25519();
    const { priv: privM } = await generateMLKEM768();
    const salt = crypto.getRandomValues(new Uint8Array(32));
    const params = { salt, time: 1, memory: 8192 };
    const masterKey = await deriveKey('correct', params);
    const encPrivX = await aesGcmEncrypt(masterKey, privX);
    const encPrivM = await aesGcmEncrypt(masterKey, privM);

    await expect(unlock('wrong', params, b64Enc(encPrivX), b64Enc(encPrivM))).rejects.toThrow();
  }, 30_000);

  it('encryptEntry + decryptEntry round-trip returns original data', async () => {
    const { priv: privX, pub: pubX } = generateX25519();
    const { priv: privM, pub: pubM } = await generateMLKEM768();

    const data = { username: 'user@example.com', password: 'secret123', notes: 'test note' };
    const session = {
      privX25519: privX,
      privMLKEM: privM,
      pubX25519: pubX,
      pubMLKEM: pubM,
      userId: 'user-123',
    };

    const encrypted = await encryptEntry(data, session);

    // Simulate what the API returns
    const apiEntry = {
      id: 'entry-1',
      owner_id: 'user-123',
      type: 'password',
      name: 'Test',
      created_at: '',
      updated_at: '',
      encrypted_data: encrypted.encrypted_data,
      data_nonce: encrypted.data_nonce,
      entry_key: encrypted.entry_keys[0],
    };

    const decrypted = await decryptEntry(apiEntry, session);
    expect(decrypted.username).toBe(data.username);
    expect(decrypted.password).toBe(data.password);
    expect(decrypted.notes).toBe(data.notes);
  });

  it('data_nonce field is 12 zero bytes (matches CLI behavior)', async () => {
    const { priv: privX, pub: pubX } = generateX25519();
    const { priv: privM, pub: pubM } = await generateMLKEM768();
    const session = {
      privX25519: privX, privMLKEM: privM, pubX25519: pubX, pubMLKEM: pubM, userId: 'u1',
    };
    const encrypted = await encryptEntry({ username: 'a' }, session);
    const nonce = b64Dec(encrypted.data_nonce);
    expect(nonce).toEqual(new Uint8Array(12)); // all zeros
  });

  it('encryptEntry produces entry_key for current user', async () => {
    const { priv: privX, pub: pubX } = generateX25519();
    const { priv: privM, pub: pubM } = await generateMLKEM768();
    const session = {
      privX25519: privX, privMLKEM: privM, pubX25519: pubX, pubMLKEM: pubM, userId: 'my-user-id',
    };
    const encrypted = await encryptEntry({ username: 'test' }, session);
    expect(encrypted.entry_keys).toHaveLength(1);
    expect(encrypted.entry_keys[0].user_id).toBe('my-user-id');
  });

  it('decryptEntry throws when entry_key is missing', async () => {
    const { priv: privX } = generateX25519();
    const { priv: privM } = await generateMLKEM768();
    const apiEntry = {
      id: 'e1', owner_id: 'u1', type: 'password', name: 'T',
      created_at: '', updated_at: '',
      encrypted_data: 'aaaa',
    };
    await expect(
      decryptEntry(apiEntry, { privX25519: privX, privMLKEM: privM }),
    ).rejects.toThrow('entry missing entry_key');
  });

  it('interop: decrypt entry created by Go', async () => {
    const vectorPath = join(__dirname, '../../../testdata/vectors.json');
    const raw = await readFile(vectorPath, 'utf-8');
    const v = JSON.parse(raw) as {
      full_entry: {
        master_password: string;
        kdf_salt: string;
        kdf_time: number;
        kdf_memory: number;
        enc_priv_x25519: string;
        enc_priv_mlkem: string;
        pub_x25519: string;
        pub_mlkem: string;
        entry_encrypted_data: string;
        entry_encrypted_key: string;
        expected_username: string;
        expected_password: string;
      };
    };
    const fe = v.full_entry;
    const { privX25519, privMLKEM } = await unlock(
      fe.master_password,
      { salt: b64Dec(fe.kdf_salt), time: fe.kdf_time, memory: fe.kdf_memory },
      fe.enc_priv_x25519,
      fe.enc_priv_mlkem,
    );
    const apiEntry = {
      id: 'e1', owner_id: 'u1', type: 'password', name: 'T',
      created_at: '', updated_at: '',
      encrypted_data: fe.entry_encrypted_data,
      entry_key: { user_id: 'u1', encrypted_key: fe.entry_encrypted_key },
    };
    const data = await decryptEntry(apiEntry, { privX25519, privMLKEM });
    expect(data.username).toBe(fe.expected_username);
    expect(data.password).toBe(fe.expected_password);
  }, 30_000);
});
