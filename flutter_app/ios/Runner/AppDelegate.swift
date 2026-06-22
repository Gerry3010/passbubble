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

import AuthenticationServices
import Flutter
import UIKit
import Security

@main
@objc class AppDelegate: FlutterAppDelegate {

  /// Shared with the AutoFill Credential Provider extension. Must match
  /// `kAppGroup` in CredentialProviderViewController.swift and the App Groups
  /// capability on both the Runner and the extension target.
  private let kAppGroup = "group.net.geraldhofbauer.passbubble"
  private let channelName = "net.geraldhofbauer.passbubble/autofill"

  override func application(
    _ application: UIApplication,
    didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]?
  ) -> Bool {
    GeneratedPluginRegistrant.register(with: self)

    if let controller = window?.rootViewController as? FlutterViewController {
      let channel = FlutterMethodChannel(name: channelName,
                                         binaryMessenger: controller.binaryMessenger)
      channel.setMethodCallHandler { [weak self] call, result in
        guard let self = self else { result(nil); return }
        switch call.method {
        // Feeds the credential-provider extension with the already-decrypted
        // credentials (JSON), written to the shared App Group keychain. Called on
        // unlock. The app owns the hybrid-KEM crypto; the extension does none.
        case "iosSyncCredentials":
          if let json = (call.arguments as? [String: Any])?["credentials"] as? String {
            self.keychainSet(account: "autofill_credentials", data: Data(json.utf8))
            self.registerIdentities(fromJSON: json)
          }
          result(nil)
        // Vault locked / logged out — wipe the shared data.
        case "iosClearVault", "clearVault":
          self.iosClearVault()
          result(nil)
        // Opens iOS Settings so the user can enable Passbubble as an AutoFill
        // provider (Settings → General → AutoFill & Passwords).
        case "requestEnable":
          self.openAppSettings()
          result(true)
        default:
          result(FlutterMethodNotImplemented)
        }
      }
    }

    return super.application(application, didFinishLaunchingWithOptions: launchOptions)
  }

  // MARK: - Shared App Group writers

  private func iosClearVault() {
    keychainDelete(account: "autofill_credentials")
    // Clean up keys written by older builds, if any.
    keychainDelete(account: "priv_x25519_plain")
    if let defaults = UserDefaults(suiteName: kAppGroup) {
      ["server_url", "access_token", "refresh_token"].forEach { defaults.removeObject(forKey: $0) }
    }
    ASCredentialIdentityStore.shared.removeAllCredentialIdentities { _, _ in }
  }

  // MARK: - AutoFill credential identities

  private struct Cred: Decodable {
    let id, name, url, username, password, totp: String
    init(from d: Decoder) throws {
      let c = try d.container(keyedBy: CodingKeys.self)
      func s(_ k: CodingKeys) -> String { (try? c.decode(String.self, forKey: k)) ?? "" }
      id = s(.id); name = s(.name); url = s(.url)
      username = s(.username); password = s(.password); totp = s(.totp)
    }
    enum CodingKeys: String, CodingKey { case id, name, url, username, password, totp }
  }

  /// Registers password + one-time-code credential identities so iOS proactively
  /// suggests Passbubble on matching login / verification-code fields (and lists
  /// it under "Set Up Verification Codes Using"). Required for OTP fill; password
  /// fill also works via the manual provider list, but this improves matching.
  private func registerIdentities(fromJSON json: String) {
    guard let data = json.data(using: .utf8),
          let creds = try? JSONDecoder().decode([Cred].self, from: data) else { return }
    guard #available(iOS 18.0, *) else { return } // generic identity API + OTP
    ASCredentialIdentityStore.shared.getState { state in
      guard state.isEnabled else { return }
      var identities: [ASCredentialIdentity] = []
      for c in creds {
        guard let host = Self.host(from: c.url) else { continue }
        let svc = ASCredentialServiceIdentifier(identifier: host, type: .domain)
        if !c.username.isEmpty || !c.password.isEmpty {
          identities.append(ASPasswordCredentialIdentity(
            serviceIdentifier: svc, user: c.username, recordIdentifier: c.id))
        }
        if !c.totp.isEmpty {
          identities.append(ASOneTimeCodeCredentialIdentity(
            serviceIdentifier: svc,
            label: c.name.isEmpty ? (c.username.isEmpty ? host : c.username) : c.name,
            recordIdentifier: c.id))
        }
      }
      Task { try? await ASCredentialIdentityStore.shared.replaceCredentialIdentities(identities) }
    }
  }

  /// Domain from a possibly scheme-less / http / local URL, stripped of "www.".
  private static func host(from urlString: String) -> String? {
    let t = urlString.trimmingCharacters(in: .whitespaces)
    guard !t.isEmpty else { return nil }
    let raw = URL(string: t)?.host ?? URL(string: "https://\(t)")?.host
    guard var h = raw, !h.isEmpty else { return nil }
    if h.hasPrefix("www.") { h.removeFirst(4) }
    return h.lowercased()
  }

  // MARK: - Shared keychain (App Group access group)

  private func keychainSet(account: String, data: Data) {
    let base: [CFString: Any] = [
      kSecClass: kSecClassGenericPassword,
      kSecAttrService: kAppGroup,
      kSecAttrAccount: account,
      kSecAttrAccessGroup: kAppGroup,
    ]
    SecItemDelete(base as CFDictionary)
    var add = base
    add[kSecValueData] = data
    add[kSecAttrAccessible] = kSecAttrAccessibleAfterFirstUnlock
    SecItemAdd(add as CFDictionary, nil)
  }

  private func keychainDelete(account: String) {
    let query: [CFString: Any] = [
      kSecClass: kSecClassGenericPassword,
      kSecAttrService: kAppGroup,
      kSecAttrAccount: account,
      kSecAttrAccessGroup: kAppGroup,
    ]
    SecItemDelete(query as CFDictionary)
  }

  private func openAppSettings() {
    if let url = URL(string: UIApplication.openSettingsURLString) {
      UIApplication.shared.open(url)
    }
  }
}
