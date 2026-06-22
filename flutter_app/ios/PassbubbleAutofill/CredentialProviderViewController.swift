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

    // MARK: - ASCredentialProviderViewController overrides

    override func prepareCredentialList(for serviceIdentifiers: [ASCredentialServiceIdentifier]) {
        self.serviceIdentifiers = serviceIdentifiers
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
        entries = Self.loadCredentials()
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
        extensionContext.completeRequest(
            withSelectedCredential: ASPasswordCredential(user: entry.username, password: entry.password))
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

    enum CodingKeys: String, CodingKey { case id, name, url, username, password }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = (try? c.decode(String.self, forKey: .id)) ?? ""
        name = (try? c.decode(String.self, forKey: .name)) ?? ""
        url = (try? c.decode(String.self, forKey: .url)) ?? ""
        username = (try? c.decode(String.self, forKey: .username)) ?? ""
        password = (try? c.decode(String.self, forKey: .password)) ?? ""
    }
}
