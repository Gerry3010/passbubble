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

import android.content.Intent
import android.net.Uri
import android.os.Build
import android.provider.Settings
import android.view.autofill.AutofillManager
import io.flutter.embedding.android.FlutterFragmentActivity
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.plugin.common.MethodChannel

class MainActivity : FlutterFragmentActivity() {

    private val channelName = "net.geraldhofbauer.passbubble/autofill"

    override fun configureFlutterEngine(flutterEngine: FlutterEngine) {
        super.configureFlutterEngine(flutterEngine)

        MethodChannel(flutterEngine.dartExecutor.binaryMessenger, channelName)
            .setMethodCallHandler { call, result ->
                when (call.method) {
                    "isSupported" -> result.success(autofillManager()?.isAutofillSupported == true)

                    "isEnabled" -> result.success(isEnabled())

                    "requestEnable" -> result.success(requestEnable())

                    "updateCredentials" -> {
                        val json = call.argument<String>("credentials")
                        if (json == null) {
                            result.error("ARG", "credentials JSON is required", null)
                        } else {
                            AutofillVaultStore.update(this, json)
                            result.success(null)
                        }
                    }

                    "clearVault" -> {
                        AutofillVaultStore.clear(this)
                        result.success(null)
                    }

                    else -> result.notImplemented()
                }
            }
    }

    private fun autofillManager(): AutofillManager? =
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O)
            getSystemService(AutofillManager::class.java)
        else null

    /** True only when Passbubble is the *currently selected* system autofill service. */
    private fun isEnabled(): Boolean {
        val am = autofillManager() ?: return false
        return am.isAutofillSupported && am.hasEnabledAutofillServices()
    }

    /** Opens the system autofill-service picker pre-targeted at this app. */
    private fun requestEnable(): Boolean {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) return false
        return try {
            val intent = Intent(Settings.ACTION_REQUEST_SET_AUTOFILL_SERVICE).apply {
                data = Uri.parse("package:$packageName")
            }
            startActivity(intent)
            true
        } catch (e: Exception) {
            // Some OEM ROMs don't expose the direct picker — fall back to settings.
            try {
                startActivity(Intent(Settings.ACTION_SETTINGS))
                true
            } catch (e2: Exception) {
                false
            }
        }
    }
}
