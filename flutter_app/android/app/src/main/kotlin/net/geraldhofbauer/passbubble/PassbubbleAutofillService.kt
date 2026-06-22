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

package net.geraldhofbauer.passbubble

import android.app.PendingIntent
import android.app.assist.AssistStructure
import android.content.Intent
import android.os.Build
import android.os.CancellationSignal
import android.service.autofill.AutofillService
import android.service.autofill.Dataset
import android.service.autofill.FillCallback
import android.service.autofill.FillRequest
import android.service.autofill.FillResponse
import android.service.autofill.SaveCallback
import android.service.autofill.SaveRequest
import android.text.InputType
import android.util.Log
import android.view.autofill.AutofillValue
import android.widget.RemoteViews
import androidx.annotation.RequiresApi
import java.net.URL
import org.json.JSONArray

/**
 * Passbubble AutoFill Service (Android 8.0+, API 26).
 *
 * It does **no** crypto or networking: the Flutter app decrypts entries on
 * unlock and writes ready-to-fill credentials into [AutofillVaultStore]. This
 * service just reads that list, matches the focused form's web domain, and
 * offers the matching logins. See AutofillVaultStore for the bridge format.
 */
@RequiresApi(Build.VERSION_CODES.O)
class PassbubbleAutofillService : AutofillService() {

    private companion object {
        const val TAG = "PassbubbleAutofill"
    }

    override fun onFillRequest(
        request: FillRequest,
        cancellationSignal: CancellationSignal,
        callback: FillCallback,
    ) {
        val context = request.fillContexts.lastOrNull() ?: run {
            callback.onSuccess(null)
            return
        }

        val parser = StructureParser(context.structure)
        parser.parse()

        if (parser.usernameIds.isEmpty() && parser.passwordIds.isEmpty()) {
            callback.onSuccess(null)
            return
        }

        val credsJson = AutofillVaultStore.prefs(this).getString(AutofillVaultStore.KEY_CREDENTIALS, null)
        if (credsJson.isNullOrEmpty()) {
            // Vault locked / nothing handed over yet — offer an "unlock" action.
            callback.onSuccess(
                runCatching { buildAuthResponse(parser, "Open Passbubble to unlock vault") }.getOrNull(),
            )
            return
        }

        val response: FillResponse? = try {
            val all = JSONArray(credsJson)
            val webDomain = parser.webDomain
            val builder = FillResponse.Builder()
            var count = 0
            for (i in 0 until all.length()) {
                if (count >= 8) break
                val o = all.getJSONObject(i)
                val url = o.optString("url", "")
                if (!webDomain.isNullOrEmpty() && !hostMatches(url, webDomain)) continue
                val username = o.optString("username", "")
                val password = o.optString("password", "")
                if (username.isEmpty() && password.isEmpty()) continue
                val label = o.optString("name", "").ifEmpty { url }
                builder.addDataset(buildDataset(parser, label, username, password))
                count++
            }
            // An empty FillResponse throws — null means "no suggestions".
            if (count > 0) builder.build() else null
        } catch (e: Exception) {
            Log.w(TAG, "fill request failed", e)
            null
        }

        callback.onSuccess(response)
    }

    override fun onSaveRequest(request: SaveRequest, callback: SaveCallback) {
        // Saving new credentials back to the vault is a future feature.
        callback.onSuccess()
    }

    // ── Matching ───────────────────────────────────────────────────────────────

    /**
     * Lenient host match between a stored entry URL (possibly scheme-less, e.g.
     * "github.com" or "www.github.com/login") and the browser's web domain.
     */
    private fun hostMatches(entryUrl: String, webDomain: String): Boolean {
        if (entryUrl.isEmpty()) return false
        val host = try {
            val normalized = if (entryUrl.contains("://")) entryUrl else "https://$entryUrl"
            URL(normalized).host?.lowercase() ?: entryUrl.lowercase()
        } catch (_: Exception) {
            entryUrl.lowercase()
        }
        val d = webDomain.lowercase().removePrefix("www.")
        val h = host.removePrefix("www.")
        if (d.isEmpty() || h.isEmpty()) return false
        return h == d || h.endsWith(".$d") || d.endsWith(".$h") || h.contains(d) || d.contains(h)
    }

    // ── Autofill response builders ─────────────────────────────────────────────

    /** Brand-styled dropdown row (dark surface + green text), readable on the
     *  system autofill popup regardless of its background. */
    private fun presentation(label: String): RemoteViews =
        RemoteViews(packageName, R.layout.autofill_item).apply {
            setTextViewText(R.id.autofill_label, label)
        }

    private fun buildDataset(
        parser: StructureParser,
        label: String,
        username: String,
        password: String,
    ): Dataset {
        val presentation = presentation(label)
        val builder = Dataset.Builder(presentation)
        for (id in parser.usernameIds) {
            builder.setValue(id, AutofillValue.forText(username), presentation)
        }
        for (id in parser.passwordIds) {
            builder.setValue(id, AutofillValue.forText(password), presentation)
        }
        return builder.build()
    }

    private fun buildAuthResponse(parser: StructureParser, label: String): FillResponse? {
        val intent = Intent(this, MainActivity::class.java).apply {
            flags = Intent.FLAG_ACTIVITY_NEW_TASK
        }
        val pi = PendingIntent.getActivity(
            this, 0, intent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
        )
        return FillResponse.Builder()
            .setAuthentication(
                (parser.usernameIds + parser.passwordIds).toTypedArray(),
                pi.intentSender,
                presentation(label),
            )
            .build()
    }

    // ── Structure parser ───────────────────────────────────────────────────────

    private enum class FieldKind { USERNAME, PASSWORD }

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
            if (webDomain.isNullOrEmpty()) {
                node.webDomain?.let { if (it.isNotEmpty()) webDomain = it }
            }

            val id = node.autofillId
            if (id != null) {
                when (classify(node)) {
                    FieldKind.USERNAME -> if (!usernameIds.contains(id)) usernameIds.add(id)
                    FieldKind.PASSWORD -> if (!passwordIds.contains(id)) passwordIds.add(id)
                    null -> {}
                }
            }

            for (i in 0 until node.childCount) {
                parseNode(node.getChildAt(i))
            }
        }

        /**
         * Classifies a node as a username/password field. Browsers in
         * compatibility mode rarely set autofillHints, so we also look at HTML
         * attributes (type / name / autocomplete) and the inputType flags.
         */
        private fun classify(node: AssistStructure.ViewNode): FieldKind? {
            node.autofillHints?.forEach { h ->
                val hl = h.lowercase()
                if (hl.contains("password")) return FieldKind.PASSWORD
                if (hl.contains("username") || hl.contains("email")) return FieldKind.USERNAME
            }

            val html = node.htmlInfo
            val isHtmlInput = html?.tag?.equals("input", ignoreCase = true) == true
            var htmlType = ""
            var nameBag = ""
            var autocomplete = ""
            html?.attributes?.forEach { attr ->
                val k = attr.first?.lowercase() ?: return@forEach
                val v = attr.second?.lowercase() ?: ""
                when (k) {
                    "type" -> htmlType = v
                    "name", "id" -> nameBag += " $v"
                    "autocomplete" -> autocomplete = v
                }
            }

            if (htmlType == "password" || autocomplete.contains("password")) return FieldKind.PASSWORD
            if (isPasswordInputType(node.inputType)) return FieldKind.PASSWORD

            val editable = isHtmlInput ||
                node.className?.contains("EditText") == true ||
                node.autofillHints != null
            if (!editable) return null

            if (autocomplete.contains("username") || autocomplete.contains("email")) return FieldKind.USERNAME
            if (htmlType == "email" || isEmailInputType(node.inputType)) return FieldKind.USERNAME

            if (htmlType.isEmpty() || htmlType == "text" || htmlType == "tel") {
                val bag = (nameBag + " " + (node.hint ?: "") + " " + (node.idEntry ?: "")).lowercase()
                if (bag.contains("user") || bag.contains("email") || bag.contains("login") ||
                    bag.contains("account") || bag.contains("benutzer") || bag.contains("anmeld")
                ) {
                    return FieldKind.USERNAME
                }
            }
            return null
        }

        private fun isPasswordInputType(type: Int): Boolean {
            val cls = type and InputType.TYPE_MASK_CLASS
            val variation = type and InputType.TYPE_MASK_VARIATION
            return (cls == InputType.TYPE_CLASS_TEXT && (
                variation == InputType.TYPE_TEXT_VARIATION_PASSWORD ||
                variation == InputType.TYPE_TEXT_VARIATION_VISIBLE_PASSWORD ||
                variation == InputType.TYPE_TEXT_VARIATION_WEB_PASSWORD)) ||
                (cls == InputType.TYPE_CLASS_NUMBER &&
                    variation == InputType.TYPE_NUMBER_VARIATION_PASSWORD)
        }

        private fun isEmailInputType(type: Int): Boolean {
            val cls = type and InputType.TYPE_MASK_CLASS
            val variation = type and InputType.TYPE_MASK_VARIATION
            return cls == InputType.TYPE_CLASS_TEXT && (
                variation == InputType.TYPE_TEXT_VARIATION_EMAIL_ADDRESS ||
                variation == InputType.TYPE_TEXT_VARIATION_WEB_EMAIL_ADDRESS)
        }
    }
}
