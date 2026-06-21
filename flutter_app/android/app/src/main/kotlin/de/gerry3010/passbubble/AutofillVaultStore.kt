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

package de.gerry3010.passbubble

import android.content.Context
import android.content.SharedPreferences
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey

/**
 * Bridge between the Flutter vault and the [PassbubbleAutofillService].
 *
 * The autofill service can't do the Passbubble crypto itself (hybrid X25519 +
 * ML-KEM-768 has no usable Kotlin implementation), so the Flutter app — which
 * already decrypts entries via its FFI/Go crypto — does the work: on unlock it
 * decrypts every entry that has a URL and writes the *ready-to-fill*
 * credentials here as a JSON array:
 *
 *   [ { "url": "...", "name": "...", "username": "...", "password": "..." }, … ]
 *
 * On lock/logout the app clears them. Storage is **at-rest encrypted**
 * ([EncryptedSharedPreferences], AES-256; AndroidKeyStore master key) — it holds
 * plaintext credentials, so it must never be a plain prefs file.
 */
object AutofillVaultStore {
    const val PREFS_NAME = "passbubble_autofill_prefs"
    const val KEY_CREDENTIALS = "credentials"

    fun prefs(context: Context): SharedPreferences {
        val masterKey = MasterKey.Builder(context)
            .setKeyScheme(MasterKey.KeyScheme.AES256_GCM)
            .build()
        return EncryptedSharedPreferences.create(
            context,
            PREFS_NAME,
            masterKey,
            EncryptedSharedPreferences.PrefKeyEncryptionScheme.AES256_SIV,
            EncryptedSharedPreferences.PrefValueEncryptionScheme.AES256_GCM,
        )
    }

    /** Stores the decrypted, ready-to-fill credentials (a JSON array string). */
    fun update(context: Context, credentialsJson: String) {
        prefs(context).edit().putString(KEY_CREDENTIALS, credentialsJson).apply()
    }

    /** Wipes the bridge — called on vault lock / logout. */
    fun clear(context: Context) {
        prefs(context).edit().clear().apply()
    }
}
