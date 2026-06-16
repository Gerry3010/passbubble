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
private let kAppGroup = "group.de.gerry3010.passbubble"

// Keychain / UserDefaults keys written by the Flutter app after unlock.
private let kServerURL    = "server_url"
private let kAccessToken  = "access_token"
private let kRefreshToken = "refresh_token"
private let kPubX25519    = "pub_x25519"
private let kPrivX25519   = "priv_x25519_plain"  // raw bytes of decrypted private key

class CredentialProviderViewController: ASCredentialProviderViewController {

    // MARK: - UI

    private let tableView = UITableView(frame: .zero, style: .insetGrouped)
    private var entries: [PassbubbleEntry] = []
    private var filteredEntries: [PassbubbleEntry] = []
    private let searchController = UISearchController(searchResultsController: nil)
    private var serviceIdentifiers: [ASCredentialServiceIdentifier] = []

    // MARK: - ASCredentialProviderViewController overrides

    override func prepareCredentialList(for serviceIdentifiers: [ASCredentialServiceIdentifier]) {
        self.serviceIdentifiers = serviceIdentifiers
        setupUI()
        Task { await loadEntries() }
    }

    override func prepareInterfaceForExtensionConfiguration() {
        setupUI()
        Task { await loadEntries() }
    }

    /// Called by the system when AutoFill can satisfy the request without showing UI
    /// (e.g. the app's stored credentials match exactly one entry).
    override func provideCredentialWithoutUserInteraction(for credentialIdentity: ASPasswordCredentialIdentity) {
        Task {
            guard let entry = await fetchEntry(id: credentialIdentity.recordIdentifier ?? ""),
                  let password = entry.password else {
                self.extensionContext.cancelRequest(withError:
                    NSError(domain: ASExtensionErrorDomain,
                            code: ASExtensionError.credentialIdentityNotFound.rawValue))
                return
            }
            let credential = ASPasswordCredential(user: entry.username ?? "", password: password)
            self.extensionContext.completeRequest(withSelectedCredential: credential)
        }
    }

    // MARK: - UI setup

    private func setupUI() {
        navigationItem.title = "Passbubble"
        navigationItem.rightBarButtonItem = UIBarButtonItem(
            barButtonSystemItem: .cancel,
            target: self,
            action: #selector(cancel)
        )

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
            NSError(domain: ASExtensionErrorDomain,
                    code: ASExtensionError.userCanceled.rawValue))
    }

    // MARK: - Data loading

    private func loadEntries() async {
        guard let serverURL = sharedString(forKey: kServerURL),
              let accessToken = sharedString(forKey: kAccessToken) else {
            showLockState()
            return
        }

        do {
            let fetched = try await fetchEntriesFromAPI(serverURL: serverURL, accessToken: accessToken)
            await MainActor.run {
                self.entries = fetched
                self.filteredEntries = self.relevantEntries()
                self.tableView.reloadData()
            }
        } catch {
            await MainActor.run { self.showError(error) }
        }
    }

    /// Returns entries matching any of the service identifiers (URL-based matching).
    private func relevantEntries() -> [PassbubbleEntry] {
        guard !serviceIdentifiers.isEmpty else { return entries }
        let hosts = serviceIdentifiers.compactMap { URL(string: $0.identifier)?.host }
        if hosts.isEmpty { return entries }
        let relevant = entries.filter { entry in
            guard let entryHost = URL(string: entry.url)?.host else { return false }
            return hosts.contains(where: { entryHost.contains($0) || $0.contains(entryHost) })
        }
        return relevant.isEmpty ? entries : relevant
    }

    private func fetchEntriesFromAPI(serverURL: String, accessToken: String) async throws -> [PassbubbleEntry] {
        let url = URL(string: "\(serverURL)/api/v1/entries")!
        var req = URLRequest(url: url, timeoutInterval: 10)
        req.setValue("Bearer \(accessToken)", forHTTPHeaderField: "Authorization")
        let (data, _) = try await URLSession.shared.data(for: req)
        let list = try JSONDecoder().decode([EntryAPIResponse].self, from: data)
        let privX25519 = sharedData(forKey: kPrivX25519)
        return list.map { PassbubbleEntry(from: $0, privX25519: privX25519) }
    }

    private func fetchEntry(id: String) async -> PassbubbleEntry? {
        guard let serverURL = sharedString(forKey: kServerURL),
              let accessToken = sharedString(forKey: kAccessToken) else { return nil }
        let url = URL(string: "\(serverURL)/api/v1/entries/\(id)")!
        var req = URLRequest(url: url, timeoutInterval: 10)
        req.setValue("Bearer \(accessToken)", forHTTPHeaderField: "Authorization")
        guard let (data, _) = try? await URLSession.shared.data(for: req),
              let entry = try? JSONDecoder().decode(EntryAPIResponse.self, from: data) else { return nil }
        let privX25519 = sharedData(forKey: kPrivX25519)
        var e = PassbubbleEntry(from: entry, privX25519: privX25519)
        await e.decryptData(privX25519: privX25519)
        return e
    }

    private func showLockState() {
        DispatchQueue.main.async {
            let label = UILabel()
            label.text = "Open Passbubble to unlock your vault first."
            label.textAlignment = .center
            label.numberOfLines = 0
            label.translatesAutoresizingMaskIntoConstraints = false
            self.view.addSubview(label)
            NSLayoutConstraint.activate([
                label.centerXAnchor.constraint(equalTo: self.view.centerXAnchor),
                label.centerYAnchor.constraint(equalTo: self.view.centerYAnchor),
                label.leadingAnchor.constraint(equalTo: self.view.leadingAnchor, constant: 24),
                label.trailingAnchor.constraint(equalTo: self.view.trailingAnchor, constant: -24),
            ])
        }
    }

    private func showError(_ error: Error) {
        let alert = UIAlertController(title: "Error", message: error.localizedDescription, preferredStyle: .alert)
        alert.addAction(UIAlertAction(title: "OK", style: .default))
        present(alert, animated: true)
    }

    // MARK: - Shared storage helpers

    private func sharedString(forKey key: String) -> String? {
        UserDefaults(suiteName: kAppGroup)?.string(forKey: key)
    }

    private func sharedData(forKey key: String) -> Data? {
        let query: [CFString: Any] = [
            kSecClass:            kSecClassGenericPassword,
            kSecAttrService:      kAppGroup,
            kSecAttrAccount:      key,
            kSecReturnData:       true,
            kSecAttrAccessGroup:  kAppGroup,
        ]
        var result: CFTypeRef?
        SecItemCopyMatching(query as CFDictionary, &result)
        return result as? Data
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
        content.secondaryText = entry.username ?? entry.url
        cell.contentConfiguration = content
        return cell
    }

    func tableView(_ tableView: UITableView, didSelectRowAt indexPath: IndexPath) {
        tableView.deselectRow(at: indexPath, animated: true)
        var entry = filteredEntries[indexPath.row]
        let privX25519 = sharedData(forKey: kPrivX25519)
        Task {
            await entry.decryptData(privX25519: privX25519)
            await MainActor.run {
                let credential = ASPasswordCredential(
                    user: entry.username ?? "",
                    password: entry.password ?? ""
                )
                self.extensionContext.completeRequest(withSelectedCredential: credential)
            }
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
                ($0.username ?? "").lowercased().contains(query) ||
                $0.url.lowercased().contains(query)
              }
        tableView.reloadData()
    }
}

// MARK: - Model types

private struct EntryAPIResponse: Codable {
    let id: String
    let name: String
    let url: String
    let type: String
    let encryptedData: String
    let dataNonce: String
    let entryKey: EntryKeyResponse?

    enum CodingKeys: String, CodingKey {
        case id, name, url, type
        case encryptedData = "encrypted_data"
        case dataNonce = "data_nonce"
        case entryKey = "entry_key"
    }
}

private struct EntryKeyResponse: Codable {
    let userId: String
    let encryptedKey: String
    enum CodingKeys: String, CodingKey {
        case userId = "user_id"
        case encryptedKey = "encrypted_key"
    }
}

private struct PassbubbleEntry {
    let id: String
    let name: String
    let url: String
    let type: String
    let encryptedData: String
    let encryptedKey: String?
    var username: String?
    var password: String?

    init(from api: EntryAPIResponse, privX25519: Data?) {
        self.id = api.id
        self.name = api.name
        self.url = api.url
        self.type = api.type
        self.encryptedData = api.encryptedData
        self.encryptedKey = api.entryKey?.encryptedKey
    }

    /// Decrypts the entry data using X25519 ECDH + AES-256-GCM.
    mutating func decryptData(privX25519: Data?) async {
        guard let privBytes = privX25519,
              let encKeyB64 = encryptedKey,
              let encKeyData = Data(base64Encoded: encKeyB64),
              let encDataData = Data(base64Encoded: encryptedData) else { return }

        // X25519 ECDH key unwrapping
        guard encKeyData.count >= 32 else { return }
        let ephPubBytes = encKeyData.prefix(32)
        let remainder = encKeyData.dropFirst(32)

        guard let privKey = try? Curve25519.KeyAgreement.PrivateKey(rawRepresentation: privBytes),
              let ephPub = try? Curve25519.KeyAgreement.PublicKey(rawRepresentation: ephPubBytes),
              let sharedSecret = try? privKey.sharedSecretFromKeyAgreement(with: ephPub) else { return }

        // Derive wrap key (same as Flutter: raw shared secret bytes as AES key)
        let wrapKeyBytes = sharedSecret.withUnsafeBytes { Data($0) }

        // Decrypt the data key: nonce(12) || ciphertext || mac(16)
        guard remainder.count >= 28 else { return }
        let nonce = remainder.prefix(12)
        let ctAndMac = remainder.dropFirst(12)
        guard let aesNonce = try? AES.GCM.Nonce(data: nonce),
              let wrapKey = try? SymmetricKey(data: wrapKeyBytes),
              let sealedBox = try? AES.GCM.SealedBox(nonce: aesNonce, ciphertext: ctAndMac.dropLast(16), tag: ctAndMac.suffix(16)),
              let dataKeyData = try? AES.GCM.open(sealedBox, using: wrapKey) else { return }

        // Decrypt the entry data: nonce(12) || ciphertext || mac(16)
        guard encDataData.count >= 28 else { return }
        let dataNonce = encDataData.prefix(12)
        let dataCtAndMac = encDataData.dropFirst(12)
        guard let dataAesNonce = try? AES.GCM.Nonce(data: dataNonce),
              let dataKey = try? SymmetricKey(data: dataKeyData),
              let dataSealedBox = try? AES.GCM.SealedBox(nonce: dataAesNonce, ciphertext: dataCtAndMac.dropLast(16), tag: dataCtAndMac.suffix(16)),
              let plainData = try? AES.GCM.open(dataSealedBox, using: dataKey),
              let json = try? JSONSerialization.jsonObject(with: plainData) as? [String: Any] else { return }

        self.username = json["username"] as? String
        self.password = json["password"] as? String
    }
}
