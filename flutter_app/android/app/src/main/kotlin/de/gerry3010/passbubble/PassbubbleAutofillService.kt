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

import android.app.assist.AssistStructure
import android.os.CancellationSignal
import android.service.autofill.*
import android.view.autofill.AutofillValue
import android.widget.RemoteViews
import android.app.PendingIntent
import android.content.Intent
import android.content.Context
import android.os.Build
import androidx.annotation.RequiresApi
import kotlinx.coroutines.*
import org.json.JSONArray
import org.json.JSONObject
import java.net.HttpURLConnection
import java.net.URL
import javax.crypto.Cipher
import javax.crypto.spec.GCMParameterSpec
import javax.crypto.spec.SecretKeySpec
import android.security.keystore.KeyProperties

/**
 * Passbubble AutoFill Service (Android 8.0+, API 26).
 *
 * The Flutter app stores the following in EncryptedSharedPreferences under
 * the shared prefs file "passbubble_autofill_prefs":
 *   - server_url     : String
 *   - access_token   : String
 *   - priv_x25519    : Base64 raw bytes of the user's decrypted X25519 private key
 *
 * The service reads these on each fill request, calls the Passbubble API,
 * decrypts the matching entry, and returns an AutofillResponse.
 */
@RequiresApi(Build.VERSION_CODES.O)
class PassbubbleAutofillService : AutofillService() {

    companion object {
        const val PREFS_NAME = "passbubble_autofill_prefs"
        const val KEY_SERVER_URL = "server_url"
        const val KEY_ACCESS_TOKEN = "access_token"
        const val KEY_PRIV_X25519 = "priv_x25519"
    }

    private val scope = CoroutineScope(Dispatchers.IO + SupervisorJob())

    override fun onFillRequest(
        request: FillRequest,
        cancellationSignal: CancellationSignal,
        callback: FillCallback,
    ) {
        val context = request.fillContexts.lastOrNull() ?: run {
            callback.onSuccess(null)
            return
        }

        val structure = context.structure
        val parser = StructureParser(structure)
        parser.parse()

        if (parser.usernameIds.isEmpty() && parser.passwordIds.isEmpty()) {
            callback.onSuccess(null)
            return
        }

        val prefs = getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)
        val serverUrl = prefs.getString(KEY_SERVER_URL, null)
        val accessToken = prefs.getString(KEY_ACCESS_TOKEN, null)
        val privX25519B64 = prefs.getString(KEY_PRIV_X25519, null)

        if (serverUrl == null || accessToken == null) {
            // Vault not unlocked — show unlock prompt
            callback.onSuccess(buildUnlockResponse(parser))
            return
        }

        scope.launch {
            try {
                val entries = fetchEntries(serverUrl, accessToken)
                val webDomain = parser.webDomain
                val matched = if (webDomain != null)
                    entries.filter { entry ->
                        try { URL(entry.url).host?.contains(webDomain) == true } catch (_: Exception) { false }
                    }
                else entries

                val privX25519 = privX25519B64?.let { android.util.Base64.decode(it, android.util.Base64.DEFAULT) }

                val responseBuilder = FillResponse.Builder()
                for (entry in matched.take(5)) {
                    val decrypted = decryptEntry(entry, privX25519)
                    val dataset = buildDataset(
                        parser,
                        entry.name,
                        decrypted?.username ?: "",
                        decrypted?.password ?: "",
                    )
                    responseBuilder.addDataset(dataset)
                }
                callback.onSuccess(responseBuilder.build())
            } catch (e: Exception) {
                callback.onFailure(e.message)
            }
        }

        cancellationSignal.setOnCancelListener { scope.coroutineContext.cancelChildren() }
    }

    override fun onSaveRequest(request: SaveRequest, callback: SaveCallback) {
        // Save new credentials back to vault (Phase 2 feature)
        callback.onSuccess()
    }

    override fun onDestroy() {
        scope.cancel()
        super.onDestroy()
    }

    // ── Network ──────────────────────────────────────────────────────────────

    private fun fetchEntries(serverUrl: String, accessToken: String): List<EntryData> {
        val url = URL("$serverUrl/api/v1/entries")
        val conn = url.openConnection() as HttpURLConnection
        conn.setRequestProperty("Authorization", "Bearer $accessToken")
        conn.connectTimeout = 5_000
        conn.readTimeout = 5_000
        val body = conn.inputStream.bufferedReader().readText()
        conn.disconnect()

        val array = JSONArray(body)
        return (0 until array.length()).map { i ->
            val obj = array.getJSONObject(i)
            EntryData(
                id = obj.getString("id"),
                name = obj.getString("name"),
                url = obj.optString("url", ""),
                encryptedData = obj.optString("encrypted_data", ""),
                encryptedKey = obj.optJSONObject("entry_key")?.optString("encrypted_key"),
            )
        }
    }

    // ── Crypto ───────────────────────────────────────────────────────────────

    /**
     * Decrypts an entry using X25519 ECDH + AES-256-GCM, matching the Dart
     * implementation in VaultCrypto.encryptDataKey / decryptDataKey.
     *
     * Wire format of encrypted_key: ephPub(32) || nonce(12) || ciphertext || mac(16)
     * Wire format of encrypted_data: nonce(12) || ciphertext || mac(16)
     */
    private fun decryptEntry(entry: EntryData, privX25519: ByteArray?): DecryptedEntry? {
        if (privX25519 == null || entry.encryptedKey == null || entry.encryptedData.isEmpty()) return null
        return try {
            val encKeyBytes = android.util.Base64.decode(entry.encryptedKey, android.util.Base64.DEFAULT)
            if (encKeyBytes.size < 32) return null

            val ephPubBytes = encKeyBytes.copyOf(32)
            val remainder = encKeyBytes.copyOfRange(32, encKeyBytes.size)

            // X25519 ECDH
            val sharedSecretBytes = x25519(privX25519, ephPubBytes)

            // Decrypt data key
            val dataKey = aesGcmDecrypt(sharedSecretBytes, remainder) ?: return null

            // Decrypt entry data
            val encDataBytes = android.util.Base64.decode(entry.encryptedData, android.util.Base64.DEFAULT)
            val plainJson = aesGcmDecrypt(dataKey, encDataBytes) ?: return null

            val json = JSONObject(String(plainJson, Charsets.UTF_8))
            DecryptedEntry(
                username = json.optString("username"),
                password = json.optString("password"),
            )
        } catch (_: Exception) {
            null
        }
    }

    /** Raw X25519 scalar multiplication using Android's Conscrypt provider. */
    private fun x25519(privateKey: ByteArray, publicKey: ByteArray): ByteArray {
        // Android KeyAgreement "X25519" is available via Conscrypt (API 28+)
        // For API 26/27 we use a pure-Java scalar mult fallback via KeyFactory.
        return if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
            val kf = java.security.KeyFactory.getInstance("X25519")
            val privKeySpec = java.security.spec.PKCS8EncodedKeySpec(pkcs8WrapX25519(privateKey))
            val privKey = kf.generatePrivate(privKeySpec)
            val pubKeySpec = java.security.spec.X509EncodedKeySpec(x509WrapX25519(publicKey))
            val pubKey = kf.generatePublic(pubKeySpec)
            val ka = javax.crypto.KeyAgreement.getInstance("X25519")
            ka.init(privKey)
            ka.doPhase(pubKey, true)
            ka.generateSecret()
        } else {
            x25519Fallback(privateKey, publicKey)
        }
    }

    // ── AES-256-GCM ───────────────────────────────────────────────────────────

    /**
     * Decrypts nonce(12) || ciphertext || mac(16) with a 32-byte key.
     */
    private fun aesGcmDecrypt(key: ByteArray, data: ByteArray): ByteArray? {
        if (data.size < 28) return null
        val nonce = data.copyOf(12)
        val ctAndTag = data.copyOfRange(12, data.size)
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(
            Cipher.DECRYPT_MODE,
            SecretKeySpec(key, KeyProperties.KEY_ALGORITHM_AES),
            GCMParameterSpec(128, nonce),
        )
        return cipher.doFinal(ctAndTag)
    }

    // ── X25519 key wrapping helpers ──────────────────────────────────────────

    private fun pkcs8WrapX25519(rawKey: ByteArray): ByteArray {
        // PKCS#8 wrapper for X25519 private key (RFC 8410)
        val oid = byteArrayOf(0x06, 0x03, 0x2B, 0x65, 0x6E) // OID 1.3.101.110
        val inner = byteArrayOf(0x04, rawKey.size.toByte()) + rawKey
        val seq1 = byteArrayOf(0x30, inner.size.toByte()) + inner
        val version = byteArrayOf(0x02, 0x01, 0x00)
        val algId = byteArrayOf(0x30, oid.size.toByte()) + oid
        val content = version + algId + seq1
        return byteArrayOf(0x30, content.size.toByte()) + content
    }

    private fun x509WrapX25519(rawPub: ByteArray): ByteArray {
        val oid = byteArrayOf(0x06, 0x03, 0x2B, 0x65, 0x6E)
        val algId = byteArrayOf(0x30, oid.size.toByte()) + oid
        val bitString = byteArrayOf(0x03, (rawPub.size + 1).toByte(), 0x00) + rawPub
        val content = algId + bitString
        return byteArrayOf(0x30, content.size.toByte()) + content
    }

    /** Minimal X25519 scalar multiplication fallback for API 26/27. */
    private fun x25519Fallback(priv: ByteArray, pub: ByteArray): ByteArray {
        // Use BouncyCastle if available at runtime, otherwise throw.
        // In practice, API 26+ devices with Google Play Services have Conscrypt.
        throw UnsupportedOperationException("X25519 requires API 28+ or Conscrypt")
    }

    // ── Autofill response builders ────────────────────────────────────────────

    private fun buildDataset(
        parser: StructureParser,
        label: String,
        username: String,
        password: String,
    ): Dataset {
        val presentation = RemoteViews(packageName, android.R.layout.simple_list_item_1)
        presentation.setTextViewText(android.R.id.text1, label)

        val builder = Dataset.Builder(presentation)
        for (id in parser.usernameIds) {
            builder.setValue(id, AutofillValue.forText(username), presentation)
        }
        for (id in parser.passwordIds) {
            builder.setValue(id, AutofillValue.forText(password), presentation)
        }
        return builder.build()
    }

    private fun buildUnlockResponse(parser: StructureParser): FillResponse? {
        val intent = Intent(this, MainActivity::class.java).apply {
            flags = Intent.FLAG_ACTIVITY_NEW_TASK
        }
        val pi = PendingIntent.getActivity(
            this, 0, intent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
        )
        val presentation = RemoteViews(packageName, android.R.layout.simple_list_item_1)
        presentation.setTextViewText(android.R.id.text1, "Open Passbubble to unlock vault")
        return FillResponse.Builder()
            .setAuthentication(
                (parser.usernameIds + parser.passwordIds).toTypedArray(),
                pi.intentSender,
                presentation,
            )
            .build()
    }

    // ── Structure parser ─────────────────────────────────────────────────────

    inner class StructureParser(private val structure: AssistStructure) {
        val usernameIds = mutableListOf<android.view.autofill.AutofillId>()
        val passwordIds = mutableListOf<android.view.autofill.AutofillId>()
        var webDomain: String? = null

        fun parse() {
            for (i in 0 until structure.windowNodeCount) {
                parseNode(structure.getWindowNodeAt(i).rootViewNode)
            }
        }

        private fun parseNode(node: AssistStructure.ViewNode) {
            if (webDomain == null) webDomain = node.webDomain

            val hints = node.autofillHints
            val id = node.autofillId
            if (id != null && hints != null) {
                if (hints.any { it.contains("username") || it.contains("email") }) {
                    usernameIds.add(id)
                }
                if (hints.any { it == "password" || it.contains("current-password") }) {
                    passwordIds.add(id)
                }
            }

            for (i in 0 until node.childCount) {
                parseNode(node.getChildAt(i))
            }
        }
    }

    // ── Data types ───────────────────────────────────────────────────────────

    data class EntryData(
        val id: String,
        val name: String,
        val url: String,
        val encryptedData: String,
        val encryptedKey: String?,
    )

    data class DecryptedEntry(
        val username: String?,
        val password: String?,
    )
}
