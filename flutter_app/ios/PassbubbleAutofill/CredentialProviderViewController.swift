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
import CryptoKit
import Security

// App Group identifier shared between the main app and this extension.
private let kAppGroup = "group.net.geraldhofbauer.passbubble"
// Keychain account holding the JSON array of already-decrypted credentials,
// written by the Flutter app on unlock (it owns the hybrid-KEM crypto). The
// extension does no crypto or networking itself.
private let kCredentials = "autofill_credentials"

class CredentialProviderViewController: ASCredentialProviderViewController {

    // MARK: - UI

    private let tableView = UITableView(frame: .zero, style: .insetGrouped)
    private var entries: [Credential] = []
    private var filteredEntries: [Credential] = []
    private let searchController = UISearchController(searchResultsController: nil)
    private var serviceIdentifiers: [ASCredentialServiceIdentifier] = []

    private enum Mode { case password, oneTimeCode }
    private var mode: Mode = .password

    // MARK: - ASCredentialProviderViewController overrides

    override func prepareCredentialList(for serviceIdentifiers: [ASCredentialServiceIdentifier]) {
        self.serviceIdentifiers = serviceIdentifiers
        self.mode = .password
        setupUI()
        loadEntries()
    }

    /// iOS 18+: the system is filling a verification-code field. Show only
    /// entries that carry a TOTP secret; on selection we return the live code.
    @available(iOS 18.0, *)
    override func prepareOneTimeCodeCredentialList(for serviceIdentifiers: [ASCredentialServiceIdentifier]) {
        self.serviceIdentifiers = serviceIdentifiers
        self.mode = .oneTimeCode
        setupUI()
        loadEntries()
    }

    override func prepareInterfaceForExtensionConfiguration() {
        setupUI()
        loadEntries()
    }

    /// Called by the system when AutoFill can satisfy the request without UI.
    override func provideCredentialWithoutUserInteraction(for credentialIdentity: ASPasswordCredentialIdentity) {
        let all = Self.loadCredentials()
        if let entry = all.first(where: { $0.id == credentialIdentity.recordIdentifier }) {
            extensionContext.completeRequest(
                withSelectedCredential: ASPasswordCredential(user: entry.username, password: entry.password))
            return
        }
        extensionContext.cancelRequest(withError:
            NSError(domain: ASExtensionErrorDomain,
                    code: ASExtensionError.credentialIdentityNotFound.rawValue))
    }

    // MARK: - UI setup

    private func setupUI() {
        navigationItem.title = "Passbubble"
        navigationItem.rightBarButtonItem = UIBarButtonItem(
            barButtonSystemItem: .cancel, target: self, action: #selector(cancel))

        searchController.searchResultsUpdater = self
        searchController.obscuresBackgroundDuringPresentation = false
        searchController.searchBar.placeholder = "Search entries…"
        navigationItem.searchController = searchController
        definesPresentationContext = true

        tableView.translatesAutoresizingMaskIntoConstraints = false
        tableView.register(UITableViewCell.self, forCellReuseIdentifier: "cell")
        tableView.dataSource = self
        tableView.delegate = self
        view.addSubview(tableView)
        NSLayoutConstraint.activate([
            tableView.topAnchor.constraint(equalTo: view.topAnchor),
            tableView.leadingAnchor.constraint(equalTo: view.leadingAnchor),
            tableView.trailingAnchor.constraint(equalTo: view.trailingAnchor),
            tableView.bottomAnchor.constraint(equalTo: view.bottomAnchor),
        ])
    }

    @objc private func cancel() {
        extensionContext.cancelRequest(withError:
            NSError(domain: ASExtensionErrorDomain, code: ASExtensionError.userCanceled.rawValue))
    }

    // MARK: - Data loading

    private func loadEntries() {
        var all = Self.loadCredentials()
        // For verification-code requests, only entries that carry a TOTP secret.
        if mode == .oneTimeCode { all = all.filter { !$0.totp.isEmpty } }
        entries = all
        if entries.isEmpty {
            showLockState()
            return
        }
        filteredEntries = relevantEntries()
        tableView.reloadData()
    }

    /// Reads + decodes the cached credentials from the shared App Group keychain.
    private static func loadCredentials() -> [Credential] {
        let query: [CFString: Any] = [
            kSecClass: kSecClassGenericPassword,
            kSecAttrService: kAppGroup,
            kSecAttrAccount: kCredentials,
            kSecReturnData: true,
            kSecAttrAccessGroup: kAppGroup,
        ]
        var result: CFTypeRef?
        guard SecItemCopyMatching(query as CFDictionary, &result) == errSecSuccess,
              let data = result as? Data,
              let creds = try? JSONDecoder().decode([Credential].self, from: data) else { return [] }
        return creds
    }

    /// Entries whose host matches one of the requested service identifiers.
    /// Falls back to the full list when nothing matches, so manual search works.
    private func relevantEntries() -> [Credential] {
        guard !serviceIdentifiers.isEmpty else { return entries }
        let hosts = serviceIdentifiers.compactMap { host(from: $0.identifier) }
        if hosts.isEmpty { return entries }
        let relevant = entries.filter { entry in
            guard let entryHost = host(from: entry.url) else { return false }
            return hosts.contains(where: { entryHost.contains($0) || $0.contains(entryHost) })
        }
        return relevant.isEmpty ? entries : relevant
    }

    /// Extracts the host, tolerating scheme-less entries ("example.com") by
    /// retrying with an "https://" prefix; strips a leading "www.".
    private func host(from urlString: String) -> String? {
        let trimmed = urlString.trimmingCharacters(in: .whitespaces)
        guard !trimmed.isEmpty else { return nil }
        let raw = URL(string: trimmed)?.host ?? URL(string: "https://\(trimmed)")?.host
        guard var h = raw, !h.isEmpty else { return nil }
        if h.hasPrefix("www.") { h.removeFirst(4) }
        return h.lowercased()
    }

    private func showLockState() {
        let label = UILabel()
        label.text = "Open Passbubble and unlock your vault first."
        label.textAlignment = .center
        label.numberOfLines = 0
        label.translatesAutoresizingMaskIntoConstraints = false
        view.addSubview(label)
        NSLayoutConstraint.activate([
            label.centerXAnchor.constraint(equalTo: view.centerXAnchor),
            label.centerYAnchor.constraint(equalTo: view.centerYAnchor),
            label.leadingAnchor.constraint(equalTo: view.leadingAnchor, constant: 24),
            label.trailingAnchor.constraint(equalTo: view.trailingAnchor, constant: -24),
        ])
    }
}

// MARK: - UITableViewDataSource / Delegate

extension CredentialProviderViewController: UITableViewDataSource, UITableViewDelegate {
    func tableView(_ tableView: UITableView, numberOfRowsInSection section: Int) -> Int {
        filteredEntries.count
    }

    func tableView(_ tableView: UITableView, cellForRowAt indexPath: IndexPath) -> UITableViewCell {
        let cell = tableView.dequeueReusableCell(withIdentifier: "cell", for: indexPath)
        let entry = filteredEntries[indexPath.row]
        var content = cell.defaultContentConfiguration()
        content.text = entry.name
        content.secondaryText = entry.username.isEmpty ? entry.url : entry.username
        cell.contentConfiguration = content
        return cell
    }

    func tableView(_ tableView: UITableView, didSelectRowAt indexPath: IndexPath) {
        tableView.deselectRow(at: indexPath, animated: true)
        let entry = filteredEntries[indexPath.row]
        if mode == .oneTimeCode, #available(iOS 18.0, *) {
            guard let code = TOTP.generate(base32Secret: entry.totp) else { cancel(); return }
            extensionContext.completeOneTimeCodeRequest(using: ASOneTimeCodeCredential(code: code))
        } else {
            extensionContext.completeRequest(
                withSelectedCredential: ASPasswordCredential(user: entry.username, password: entry.password))
        }
    }
}

// MARK: - UISearchResultsUpdating

extension CredentialProviderViewController: UISearchResultsUpdating {
    func updateSearchResults(for searchController: UISearchController) {
        let query = searchController.searchBar.text?.lowercased() ?? ""
        filteredEntries = query.isEmpty
            ? relevantEntries()
            : entries.filter {
                $0.name.lowercased().contains(query) ||
                $0.username.lowercased().contains(query) ||
                $0.url.lowercased().contains(query)
              }
        tableView.reloadData()
    }
}

// MARK: - Model

private struct Credential: Codable {
    let id: String
    let name: String
    let url: String
    let username: String
    let password: String
    let totp: String   // base32 TOTP secret, "" if none

    enum CodingKeys: String, CodingKey { case id, name, url, username, password, totp }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = (try? c.decode(String.self, forKey: .id)) ?? ""
        name = (try? c.decode(String.self, forKey: .name)) ?? ""
        url = (try? c.decode(String.self, forKey: .url)) ?? ""
        username = (try? c.decode(String.self, forKey: .username)) ?? ""
        password = (try? c.decode(String.self, forKey: .password)) ?? ""
        totp = (try? c.decode(String.self, forKey: .totp)) ?? ""
    }
}

// MARK: - TOTP (SHA1, 6 digits, 30s period — matches the Flutter app)

private enum TOTP {
    static func generate(base32Secret: String, at date: Date = Date()) -> String? {
        guard let key = base32Decode(base32Secret) else { return nil }
        var counter = UInt64(date.timeIntervalSince1970 / 30).bigEndian
        let counterData = Data(bytes: &counter, count: 8)
        let mac = HMAC<Insecure.SHA1>.authenticationCode(for: counterData, using: SymmetricKey(data: key))
        let hash = Data(mac)
        let offset = Int(hash[hash.count - 1] & 0x0f)
        let binary = (UInt32(hash[offset] & 0x7f) << 24)
            | (UInt32(hash[offset + 1]) << 16)
            | (UInt32(hash[offset + 2]) << 8)
            | UInt32(hash[offset + 3])
        return String(format: "%06u", binary % 1_000_000)
    }

    /// RFC 4648 base32 decode (uppercase, no padding required), tolerant of
    /// lowercase, spaces and "=" padding as stored in entries.
    private static func base32Decode(_ input: String) -> Data? {
        let alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"
        let clean = input.uppercased().filter { $0 != "=" && $0 != " " }
        guard !clean.isEmpty else { return nil }
        var bits = 0, value = 0
        var out = [UInt8]()
        for ch in clean {
            guard let idx = alphabet.firstIndex(of: ch) else { return nil }
            value = (value << 5) | alphabet.distance(from: alphabet.startIndex, to: idx)
            bits += 5
            if bits >= 8 {
                bits -= 8
                out.append(UInt8((value >> bits) & 0xff))
            }
        }
        return Data(out)
    }
}
