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

package vault

import (
	"fmt"

	"github.com/Gerry3010/passbubble/cli/internal/apiclient"
	"github.com/Gerry3010/passbubble/cli/internal/crypto"
)

// mlkem768PubLen is the ML-KEM-768 encapsulation (public) key size. An account
// created by the X25519-only Flutter app stores a 32-byte placeholder here, so a
// length mismatch tells us the account has no real post-quantum key yet.
const mlkem768PubLen = 1184

// NeedsKeyUpgrade reports whether the account is still X25519-only (its stored
// ML-KEM public key is a placeholder rather than a real 1184-byte key).
func (v *Vault) NeedsKeyUpgrade() bool {
	pub, err := crypto.B64Dec(v.cfg.PubMLKEM768)
	if err != nil {
		return true
	}
	return len(pub) != mlkem768PubLen
}

// UpgradeResult summarises a post-quantum key upgrade.
type UpgradeResult struct {
	Rewrapped int      // entries successfully re-wrapped to hybrid
	Failed    []string // entry IDs that could not be re-wrapped (left untouched)
}

// UpgradeToHybrid retrofits a real ML-KEM-768 keypair onto an X25519-only
// account and re-wraps every owned entry's data key to the hybrid (X25519 +
// ML-KEM) format.
//
// The existing X25519 keypair is kept deliberately: entries that other users
// shared to our X25519 public key must remain decryptable, so only the ML-KEM
// half is generated anew. Legacy (X25519-only) entry keys decrypt with X25519
// alone, so any entry that fails to re-wrap stays readable — the upgrade is safe
// to re-run.
func (v *Vault) UpgradeToHybrid(masterPassword string) (*UpgradeResult, error) {
	masterKey, err := v.deriveMasterKey(masterPassword)
	if err != nil {
		return nil, err
	}
	// Load the current (old) private keys; also verifies the master password.
	if err := v.decryptPrivKeys(masterKey); err != nil {
		return nil, fmt.Errorf("wrong master password")
	}
	oldPrivX25519 := v.privX25519
	oldPrivMLKEM := v.privMLKEM

	pubX25519, err := crypto.B64Dec(v.cfg.PubX25519)
	if err != nil {
		return nil, fmt.Errorf("decode pub_x25519: %w", err)
	}

	// Generate a fresh ML-KEM-768 keypair and encrypt its private key at rest.
	newPrivMLKEM, newPubMLKEM, err := crypto.GenerateMLKEM768()
	if err != nil {
		return nil, fmt.Errorf("generate ml-kem keypair: %w", err)
	}
	encNewPrivMLKEM, err := crypto.Encrypt(masterKey, newPrivMLKEM)
	if err != nil {
		return nil, fmt.Errorf("encrypt ml-kem private key: %w", err)
	}

	// Persist the new public + encrypted-private key material server-side. X25519
	// is unchanged, only the ML-KEM half is replaced.
	encNewPrivMLKEMB64 := crypto.B64Enc(encNewPrivMLKEM)
	newPubMLKEMB64 := crypto.B64Enc(newPubMLKEM)
	if err := v.client.UpdateUserKeys(apiclient.UpdateKeysRequest{
		PubX25519:       v.cfg.PubX25519,
		PubMLKEM768:     newPubMLKEMB64,
		EncPrivX25519:   v.cfg.EncPrivX25519,
		EncPrivMLKEM768: encNewPrivMLKEMB64,
	}); err != nil {
		return nil, fmt.Errorf("update keys on server: %w", err)
	}

	// Update local config + in-memory key so subsequent operations use the new key.
	v.cfg.PubMLKEM768 = newPubMLKEMB64
	v.cfg.EncPrivMLKEM768 = encNewPrivMLKEMB64
	_ = v.cfg.Save(v.cfgPath)
	v.privMLKEM = newPrivMLKEM

	// Re-wrap every owned entry's data key to hybrid. Decrypt with the OLD keys
	// (legacy entries only need X25519), re-encrypt to (X25519, new ML-KEM).
	res := &UpgradeResult{}
	entries, err := v.client.ListEntries()
	if err != nil {
		return res, fmt.Errorf("list entries: %w", err)
	}
	for _, e := range entries {
		full, err := v.client.GetEntry(e.ID)
		if err != nil || full.EntryKey == nil {
			res.Failed = append(res.Failed, e.ID)
			continue
		}
		encKey, err := crypto.B64Dec(full.EntryKey.EncryptedKey)
		if err != nil {
			res.Failed = append(res.Failed, e.ID)
			continue
		}
		dataKey, err := crypto.DecryptDataKey(encKey, oldPrivX25519, oldPrivMLKEM)
		if err != nil {
			res.Failed = append(res.Failed, e.ID)
			continue
		}
		newEncKey, err := crypto.EncryptDataKey(dataKey, pubX25519, newPubMLKEM)
		if err != nil {
			res.Failed = append(res.Failed, e.ID)
			continue
		}
		// Only re-wrap the key; EncryptedData "" keeps the existing ciphertext.
		// FolderID is echoed because the backend always overwrites folder_id.
		if _, err := v.client.UpdateEntry(e.ID, apiclient.UpdateEntryRequest{
			FolderID: full.FolderID,
			EntryKeys: []apiclient.EntryKey{
				{UserID: v.cfg.UserID, EncryptedKey: crypto.B64Enc(newEncKey)},
			},
		}); err != nil {
			res.Failed = append(res.Failed, e.ID)
			continue
		}
		res.Rewrapped++
	}
	return res, nil
}
