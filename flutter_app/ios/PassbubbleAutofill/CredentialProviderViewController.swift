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
import UIKit
import UniformTypeIdentifiers

// App Group identifier shared between the main app and this extension.
private let kAppGroup = "group.net.geraldhofbauer.passbubble"
// Keychain account holding the JSON array of already-decrypted credentials,
// written by the Flutter app on unlock (it owns the hybrid-KEM crypto). The
// extension does no crypto or networking itself.
private let kCredentials = "autofill_credentials"
// Copied values expire from the (local-only) pasteboard after this many seconds.
private let kClipboardTTL: TimeInterval = 60

// MARK: - Request mode

/// What the system asked us for. Decides what a tap on a value does.
enum ProviderMode {
    /// Login form: a tap fills username + password.
    case password
    /// Verification-code field (iOS 18+): a tap fills the live TOTP code.
    case oneTimeCode
    /// Context menu "AutoFill → Passwords" on any text field (iOS 18+): a tap
    /// inserts exactly the tapped value.
    case textInsert
}

/// Callbacks from the list/detail screens back to the provider controller,
/// which is the only one allowed to talk to the extension context.
protocol ProviderActions: AnyObject {
    var mode: ProviderMode { get }
    func fillLogin(_ entry: Credential)
    func fillOneTimeCode(_ entry: Credential)
    func insertText(_ text: String)
    func cancelRequest()
}

// MARK: - Provider controller

class CredentialProviderViewController: ASCredentialProviderViewController, ProviderActions {

    private(set) var mode: ProviderMode = .password
    private var serviceIdentifiers: [ASCredentialServiceIdentifier] = []
    private var navController: UINavigationController?

    // MARK: ASCredentialProviderViewController overrides

    override func prepareCredentialList(for serviceIdentifiers: [ASCredentialServiceIdentifier]) {
        self.serviceIdentifiers = serviceIdentifiers
        mode = .password
        showList()
    }

    /// iOS 18+: the system is filling a verification-code field. Show only
    /// entries that carry a TOTP secret; on selection we return the live code.
    @available(iOS 18.0, *)
    override func prepareOneTimeCodeCredentialList(for serviceIdentifiers: [ASCredentialServiceIdentifier]) {
        self.serviceIdentifiers = serviceIdentifiers
        mode = .oneTimeCode
        showList()
    }

    /// iOS 18+: the user picked "AutoFill → Passwords" from a text field's
    /// context menu. Any text value of any entry may be inserted.
    @available(iOS 18.0, *)
    override func prepareInterfaceForUserChoosingTextToInsert() {
        serviceIdentifiers = []
        mode = .textInsert
        showList()
    }

    override func prepareInterfaceForExtensionConfiguration() {
        mode = .password
        showList()
    }

    /// Legacy (pre-iOS 17) password-only direct provide.
    override func provideCredentialWithoutUserInteraction(for credentialIdentity: ASPasswordCredentialIdentity) {
        if let entry = Credential.loadAll().first(where: { $0.id == credentialIdentity.recordIdentifier }) {
            extensionContext.completeRequest(
                withSelectedCredential: ASPasswordCredential(user: entry.username, password: entry.password))
            return
        }
        cancel(with: .credentialIdentityNotFound)
    }

    /// iOS 17+: request-based direct provide. Invoked when the user taps a
    /// QuickType suggestion. Handles passwords and (iOS 18+) one-time codes —
    /// without this, tapping an OTP suggestion does nothing.
    @available(iOS 17.0, *)
    override func provideCredentialWithoutUserInteraction(for credentialRequest: ASCredentialRequest) {
        let recordId = credentialRequest.credentialIdentity.recordIdentifier
        guard let entry = Credential.loadAll().first(where: { $0.id == recordId }) else {
            cancel(with: .credentialIdentityNotFound)
            return
        }
        if #available(iOS 18.0, *), credentialRequest.type == .oneTimeCode {
            guard let code = entry.otp?.code() else {
                cancel(with: .credentialIdentityNotFound)
                return
            }
            extensionContext.completeOneTimeCodeRequest(using: ASOneTimeCodeCredential(code: code))
        } else {
            extensionContext.completeRequest(
                withSelectedCredential: ASPasswordCredential(user: entry.username, password: entry.password))
        }
    }

    /// iOS 17+: invoked when the system wants our UI to provide a specific
    /// credential. We hold the data already, so fulfil it directly when we can
    /// resolve the identity; otherwise fall back to the browse list rather than
    /// silently cancelling.
    @available(iOS 17.0, *)
    override func prepareInterfaceToProvideCredential(for credentialRequest: ASCredentialRequest) {
        let recordId = credentialRequest.credentialIdentity.recordIdentifier
        if Credential.loadAll().contains(where: { $0.id == recordId }) {
            provideCredentialWithoutUserInteraction(for: credentialRequest)
            return
        }
        if #available(iOS 18.0, *), credentialRequest.type == .oneTimeCode {
            mode = .oneTimeCode
        } else {
            mode = .password
        }
        serviceIdentifiers = [credentialRequest.credentialIdentity.serviceIdentifier]
        showList()
    }

    // MARK: UI

    /// Embeds the list in a real navigation controller. The provider controller
    /// itself is not inside one, so anything set on its own `navigationItem`
    /// (search bar, title, close button) never appears on screen.
    private func showList() {
        let list = EntryListViewController(
            entries: Credential.loadAll(),
            hosts: serviceIdentifiers.compactMap { Credential.host(from: $0.identifier) },
            actions: self)

        if let nav = navController {
            nav.setViewControllers([list], animated: false)
            return
        }
        let nav = UINavigationController(rootViewController: list)
        nav.navigationBar.prefersLargeTitles = false
        addChild(nav)
        nav.view.translatesAutoresizingMaskIntoConstraints = false
        view.addSubview(nav.view)
        NSLayoutConstraint.activate([
            nav.view.topAnchor.constraint(equalTo: view.topAnchor),
            nav.view.leadingAnchor.constraint(equalTo: view.leadingAnchor),
            nav.view.trailingAnchor.constraint(equalTo: view.trailingAnchor),
            nav.view.bottomAnchor.constraint(equalTo: view.bottomAnchor),
        ])
        nav.didMove(toParent: self)
        navController = nav
    }

    // MARK: ProviderActions

    func fillLogin(_ entry: Credential) {
        extensionContext.completeRequest(
            withSelectedCredential: ASPasswordCredential(user: entry.username, password: entry.password))
    }

    func fillOneTimeCode(_ entry: Credential) {
        guard #available(iOS 18.0, *), let code = entry.otp?.code() else {
            cancel(with: .failed)
            return
        }
        extensionContext.completeOneTimeCodeRequest(using: ASOneTimeCodeCredential(code: code))
    }

    func insertText(_ text: String) {
        guard #available(iOS 18.0, *) else { return }
        extensionContext.completeRequest(withTextToInsert: text)
    }

    func cancelRequest() {
        cancel(with: .userCanceled)
    }

    private func cancel(with code: ASExtensionError.Code) {
        extensionContext.cancelRequest(withError:
            NSError(domain: ASExtensionErrorDomain, code: code.rawValue))
    }
}

// MARK: - Entry list

final class EntryListViewController: UITableViewController, UISearchResultsUpdating {

    private struct Section {
        let title: String
        let index: String?
        let entries: [Credential]
    }

    private let allEntries: [Credential]
    private let hosts: [String]
    private weak var actions: ProviderActions?
    private let searchController = UISearchController(searchResultsController: nil)
    private var sections: [Section] = []

    init(entries: [Credential], hosts: [String], actions: ProviderActions) {
        let mode = actions.mode
        // Verification-code requests only make sense for entries with a TOTP.
        let usable = mode == .oneTimeCode ? entries.filter { $0.otp != nil } : entries
        self.allEntries = usable.sorted {
            $0.name.localizedStandardCompare($1.name) == .orderedAscending
        }
        self.hosts = hosts
        self.actions = actions
        super.init(style: .insetGrouped)
    }

    required init?(coder: NSCoder) { fatalError("not used") }

    override func viewDidLoad() {
        super.viewDidLoad()
        title = "Passbubble"
        navigationItem.rightBarButtonItem = UIBarButtonItem(
            barButtonSystemItem: .close, target: self, action: #selector(close))

        searchController.searchResultsUpdater = self
        searchController.obscuresBackgroundDuringPresentation = false
        searchController.hidesNavigationBarDuringPresentation = false
        searchController.searchBar.placeholder = "Search name, user, website, notes"
        searchController.searchBar.autocapitalizationType = .none
        navigationItem.searchController = searchController
        navigationItem.hidesSearchBarWhenScrolling = false
        definesPresentationContext = true

        tableView.register(EntryCell.self, forCellReuseIdentifier: EntryCell.reuseId)
        tableView.keyboardDismissMode = .onDrag
        rebuild(query: "")
    }

    @objc private func close() {
        actions?.cancelRequest()
    }

    // MARK: Sections

    private func rebuild(query: String) {
        let q = query.trimmingCharacters(in: .whitespaces).lowercased()
        if !q.isEmpty {
            let hits = allEntries.filter { $0.matches(query: q) }
            sections = hits.isEmpty ? [] : [Section(title: "\(hits.count) results", index: nil, entries: hits)]
        } else {
            var result: [Section] = []
            let suggested = allEntries.filter { $0.matches(hosts: hosts) }
            if !suggested.isEmpty {
                let site = hosts.first ?? "this website"
                result.append(Section(title: "Suggestions for \(site)", index: "★", entries: suggested))
            }
            // Everything else, grouped A–Z (non-letters under "#").
            let grouped = Dictionary(grouping: allEntries) { $0.indexLetter }
            for letter in grouped.keys.sorted(by: { a, b in
                if a == "#" { return false }
                if b == "#" { return true }
                return a < b
            }) {
                result.append(Section(title: letter, index: letter, entries: grouped[letter] ?? []))
            }
            sections = result
        }
        tableView.reloadData()
        updateEmptyState(searching: !q.isEmpty)
    }

    private func updateEmptyState(searching: Bool) {
        guard sections.isEmpty else {
            tableView.backgroundView = nil
            return
        }
        let label = UILabel()
        label.numberOfLines = 0
        label.textAlignment = .center
        label.textColor = .secondaryLabel
        if searching {
            label.text = "No matching entries."
        } else if actions?.mode == .oneTimeCode {
            label.text = "No entries with a 2FA code.\nAdd one in Passbubble, then open the app once."
        } else {
            label.text = "Open Passbubble and unlock your vault first."
        }
        let container = UIView()
        label.translatesAutoresizingMaskIntoConstraints = false
        container.addSubview(label)
        NSLayoutConstraint.activate([
            label.centerYAnchor.constraint(equalTo: container.centerYAnchor, constant: -60),
            label.leadingAnchor.constraint(equalTo: container.leadingAnchor, constant: 32),
            label.trailingAnchor.constraint(equalTo: container.trailingAnchor, constant: -32),
        ])
        tableView.backgroundView = container
    }

    // MARK: UISearchResultsUpdating

    func updateSearchResults(for searchController: UISearchController) {
        rebuild(query: searchController.searchBar.text ?? "")
    }

    // MARK: Table

    override func numberOfSections(in tableView: UITableView) -> Int { sections.count }

    override func tableView(_ tableView: UITableView, numberOfRowsInSection section: Int) -> Int {
        sections[section].entries.count
    }

    override func tableView(_ tableView: UITableView, titleForHeaderInSection section: Int) -> String? {
        sections[section].title
    }

    override func sectionIndexTitles(for tableView: UITableView) -> [String]? {
        let titles = sections.compactMap { $0.index }
        return titles.count > 4 ? titles : nil
    }

    override func tableView(_ tableView: UITableView, sectionForSectionIndexTitle title: String, at index: Int) -> Int {
        sections.firstIndex { $0.index == title } ?? 0
    }

    override func tableView(_ tableView: UITableView, cellForRowAt indexPath: IndexPath) -> UITableViewCell {
        let cell = tableView.dequeueReusableCell(withIdentifier: EntryCell.reuseId, for: indexPath)
        let entry = sections[indexPath.section].entries[indexPath.row]
        var content = UIListContentConfiguration.subtitleCell()
        content.text = entry.name.isEmpty ? (Credential.host(from: entry.url) ?? "Untitled") : entry.name
        let secondary = [entry.username, Credential.host(from: entry.url) ?? ""]
            .filter { !$0.isEmpty }
        content.secondaryText = secondary.joined(separator: " · ")
        content.secondaryTextProperties.color = .secondaryLabel
        content.image = UIImage(systemName: entry.otp != nil ? "key.viewfinder" : "key.fill")
        content.imageProperties.tintColor = .systemTeal
        cell.contentConfiguration = content
        cell.accessoryType = .detailButton
        return cell
    }

    override func tableView(_ tableView: UITableView, didSelectRowAt indexPath: IndexPath) {
        tableView.deselectRow(at: indexPath, animated: true)
        let entry = sections[indexPath.section].entries[indexPath.row]
        switch actions?.mode ?? .password {
        case .password: actions?.fillLogin(entry)
        case .oneTimeCode: actions?.fillOneTimeCode(entry)
        // Inserting arbitrary text: the user has to pick which value.
        case .textInsert: openDetail(entry)
        }
    }

    override func tableView(_ tableView: UITableView, accessoryButtonTappedForRowWith indexPath: IndexPath) {
        openDetail(sections[indexPath.section].entries[indexPath.row])
    }

    private func openDetail(_ entry: Credential) {
        guard let actions = actions else { return }
        navigationController?.pushViewController(
            EntryDetailViewController(entry: entry, actions: actions), animated: true)
    }
}

private final class EntryCell: UITableViewCell {
    static let reuseId = "entry"
    override init(style: UITableViewCell.CellStyle, reuseIdentifier: String?) {
        super.init(style: .subtitle, reuseIdentifier: reuseIdentifier)
    }
    required init?(coder: NSCoder) { fatalError("not used") }
}

// MARK: - Entry detail

final class EntryDetailViewController: UITableViewController {

    private enum Kind { case username, password, otp, url, notes, custom }

    private struct Row {
        let kind: Kind
        let label: String
        let value: String
        let secret: Bool
    }

    private let entry: Credential
    private weak var actions: ProviderActions?
    private var rows: [Row] = []
    private var revealed = Set<Int>()
    private var timer: Timer?

    init(entry: Credential, actions: ProviderActions) {
        self.entry = entry
        self.actions = actions
        super.init(style: .insetGrouped)
        var r: [Row] = []
        if !entry.username.isEmpty { r.append(Row(kind: .username, label: "Username", value: entry.username, secret: false)) }
        if !entry.password.isEmpty { r.append(Row(kind: .password, label: "Password", value: entry.password, secret: true)) }
        if entry.otp != nil { r.append(Row(kind: .otp, label: "One-time code", value: "", secret: false)) }
        if !entry.url.isEmpty { r.append(Row(kind: .url, label: "Website", value: entry.url, secret: false)) }
        for field in entry.fields where !field.value.isEmpty {
            r.append(Row(kind: .custom, label: field.label.isEmpty ? "Field" : field.label,
                         value: field.value, secret: field.isSecret))
        }
        if !entry.notes.isEmpty { r.append(Row(kind: .notes, label: "Notes", value: entry.notes, secret: false)) }
        rows = r
    }

    required init?(coder: NSCoder) { fatalError("not used") }

    override func viewDidLoad() {
        super.viewDidLoad()
        title = entry.name.isEmpty ? "Entry" : entry.name
        navigationItem.rightBarButtonItem = UIBarButtonItem(
            barButtonSystemItem: .close, target: self, action: #selector(close))
        tableView.register(UITableViewCell.self, forCellReuseIdentifier: "field")
    }

    override func viewWillAppear(_ animated: Bool) {
        super.viewWillAppear(animated)
        guard entry.otp != nil, timer == nil else { return }
        let t = Timer(timeInterval: 1, repeats: true) { [weak self] _ in self?.refreshOtpRow() }
        RunLoop.main.add(t, forMode: .common)
        timer = t
    }

    override func viewWillDisappear(_ animated: Bool) {
        super.viewWillDisappear(animated)
        timer?.invalidate()
        timer = nil
    }

    @objc private func close() {
        actions?.cancelRequest()
    }

    private func refreshOtpRow() {
        guard let i = rows.firstIndex(where: { $0.kind == .otp }),
              let cell = tableView.cellForRow(at: IndexPath(row: i, section: 0)) else { return }
        configure(cell, row: i)
    }

    // MARK: Table

    override func tableView(_ tableView: UITableView, numberOfRowsInSection section: Int) -> Int { rows.count }

    override func tableView(_ tableView: UITableView, titleForFooterInSection section: Int) -> String? {
        switch actions?.mode ?? .password {
        case .password: return "Tap username or password to fill both. Tap any other value to copy it — copies stay on this device and clear after 60 seconds."
        case .oneTimeCode: return "Tap the one-time code to fill it. Other values are copied and clear after 60 seconds."
        case .textInsert: return "Tap a value to insert it into the field."
        }
    }

    override func tableView(_ tableView: UITableView, cellForRowAt indexPath: IndexPath) -> UITableViewCell {
        let cell = tableView.dequeueReusableCell(withIdentifier: "field", for: indexPath)
        configure(cell, row: indexPath.row)
        return cell
    }

    private func configure(_ cell: UITableViewCell, row i: Int) {
        let row = rows[i]
        var content = UIListContentConfiguration.subtitleCell()
        // Small label on top, the value below — like the Passwords app.
        content.text = row.label
        content.textProperties.font = .preferredFont(forTextStyle: .caption1)
        content.textProperties.color = .secondaryLabel
        content.secondaryTextProperties.font = .preferredFont(forTextStyle: .body)
        content.secondaryTextProperties.color = .label
        content.secondaryTextProperties.numberOfLines = row.kind == .notes ? 0 : 1

        switch row.kind {
        case .otp:
            if let otp = entry.otp, let code = otp.code() {
                let remaining = otp.secondsRemaining()
                content.secondaryText = Self.groupDigits(code)
                content.secondaryTextProperties.font = .monospacedDigitSystemFont(ofSize: 22, weight: .semibold)
                content.text = "One-time code · \(remaining)s"
                content.textProperties.color = remaining <= 5 ? .systemOrange : .secondaryLabel
            } else {
                content.secondaryText = "Invalid secret"
            }
        default:
            if row.secret && !revealed.contains(i) {
                content.secondaryText = String(repeating: "•", count: min(max(row.value.count, 8), 16))
            } else {
                content.secondaryText = row.value
            }
            if row.secret && revealed.contains(i) {
                content.secondaryTextProperties.font = .monospacedSystemFont(ofSize: 17, weight: .regular)
            }
        }
        cell.contentConfiguration = content
        cell.accessoryView = accessoryButtons(for: i)
    }

    private func accessoryButtons(for i: Int) -> UIView {
        let stack = UIStackView()
        stack.axis = .horizontal
        stack.spacing = 14
        if rows[i].secret {
            stack.addArrangedSubview(iconButton(revealed.contains(i) ? "eye.slash" : "eye", tag: i,
                                                action: #selector(toggleReveal(_:)), label: "Show"))
        }
        stack.addArrangedSubview(iconButton("doc.on.doc", tag: i, action: #selector(copyTapped(_:)), label: "Copy"))
        stack.frame = CGRect(origin: .zero, size: stack.systemLayoutSizeFitting(UIView.layoutFittingCompressedSize))
        return stack
    }

    private func iconButton(_ symbol: String, tag: Int, action: Selector, label: String) -> UIButton {
        let b = UIButton(type: .system)
        b.setImage(UIImage(systemName: symbol), for: .normal)
        b.tag = tag
        b.accessibilityLabel = label
        b.addTarget(self, action: action, for: .touchUpInside)
        b.widthAnchor.constraint(equalToConstant: 30).isActive = true
        b.heightAnchor.constraint(equalToConstant: 30).isActive = true
        return b
    }

    @objc private func toggleReveal(_ sender: UIButton) {
        let i = sender.tag
        if revealed.contains(i) { revealed.remove(i) } else { revealed.insert(i) }
        tableView.reloadRows(at: [IndexPath(row: i, section: 0)], with: .none)
    }

    @objc private func copyTapped(_ sender: UIButton) {
        copy(row: sender.tag)
    }

    override func tableView(_ tableView: UITableView, didSelectRowAt indexPath: IndexPath) {
        tableView.deselectRow(at: indexPath, animated: true)
        let row = rows[indexPath.row]
        let mode = actions?.mode ?? .password
        switch (mode, row.kind) {
        case (.password, .username), (.password, .password):
            actions?.fillLogin(entry)
        case (.oneTimeCode, .otp):
            actions?.fillOneTimeCode(entry)
        case (.textInsert, _):
            if let text = value(of: indexPath.row) { actions?.insertText(text) }
        default:
            copy(row: indexPath.row)
        }
    }

    // MARK: Copy

    private func value(of i: Int) -> String? {
        let row = rows[i]
        if row.kind == .otp { return entry.otp?.code() }
        return row.value
    }

    private func copy(row i: Int) {
        guard let text = value(of: i), !text.isEmpty else { return }
        // Local-only (not via Universal Clipboard) and expiring, so secrets do
        // not linger on the pasteboard or show up on the Mac.
        UIPasteboard.general.setItems(
            [[UTType.plainText.identifier: text]],
            options: [.localOnly: true, .expirationDate: Date().addingTimeInterval(kClipboardTTL)])
        UINotificationFeedbackGenerator().notificationOccurred(.success)
        showToast("\(rows[i].label) copied · clears in \(Int(kClipboardTTL)) s")
    }

    private func showToast(_ text: String) {
        guard let host = navigationController?.view ?? view else { return }
        let label = PaddedLabel()
        label.text = text
        label.font = .preferredFont(forTextStyle: .subheadline)
        label.textColor = .white
        label.backgroundColor = UIColor.black.withAlphaComponent(0.8)
        label.layer.cornerRadius = 14
        label.layer.masksToBounds = true
        label.alpha = 0
        label.translatesAutoresizingMaskIntoConstraints = false
        host.addSubview(label)
        NSLayoutConstraint.activate([
            label.centerXAnchor.constraint(equalTo: host.centerXAnchor),
            label.bottomAnchor.constraint(equalTo: host.safeAreaLayoutGuide.bottomAnchor, constant: -24),
            label.leadingAnchor.constraint(greaterThanOrEqualTo: host.leadingAnchor, constant: 24),
        ])
        UIView.animate(withDuration: 0.2, animations: { label.alpha = 1 }) { _ in
            UIView.animate(withDuration: 0.3, delay: 1.6, options: [], animations: { label.alpha = 0 }) { _ in
                label.removeFromSuperview()
            }
        }
    }

    private static func groupDigits(_ code: String) -> String {
        guard code.count == 6 || code.count == 8 else { return code }
        let mid = code.index(code.startIndex, offsetBy: code.count / 2)
        return "\(code[..<mid]) \(code[mid...])"
    }
}

private final class PaddedLabel: UILabel {
    private let insets = UIEdgeInsets(top: 8, left: 14, bottom: 8, right: 14)
    override func drawText(in rect: CGRect) { super.drawText(in: rect.inset(by: insets)) }
    override var intrinsicContentSize: CGSize {
        let s = super.intrinsicContentSize
        return CGSize(width: s.width + insets.left + insets.right, height: s.height + insets.top + insets.bottom)
    }
}

// MARK: - Model

/// One entry as cached by the Flutter app (`iosSyncCredentials`). Every field
/// is optional on decode so older and newer payloads both load.
struct Credential: Decodable {
    struct Field: Decodable {
        let label: String
        let type: String
        let value: String

        var isSecret: Bool {
            ["password", "hidden", "secret", "pin", "totp"].contains(type.lowercased())
        }

        enum CodingKeys: String, CodingKey { case label, name, type, value }

        init(from decoder: Decoder) throws {
            let c = try decoder.container(keyedBy: CodingKeys.self)
            label = (try? c.decode(String.self, forKey: .label))
                ?? (try? c.decode(String.self, forKey: .name)) ?? ""
            type = (try? c.decode(String.self, forKey: .type)) ?? "text"
            value = (try? c.decode(String.self, forKey: .value)) ?? ""
        }
    }

    let id: String
    let name: String
    let url: String
    let type: String
    let username: String
    let password: String
    let notes: String
    let fields: [Field]
    let favorite: Bool
    let otp: TOTP?

    enum CodingKeys: String, CodingKey {
        case id, name, url, type, username, password, totp, notes, fields, favorite
        case totpAlgorithm = "totp_algorithm", totpDigits = "totp_digits", totpPeriod = "totp_period"
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = (try? c.decode(String.self, forKey: .id)) ?? ""
        name = (try? c.decode(String.self, forKey: .name)) ?? ""
        url = (try? c.decode(String.self, forKey: .url)) ?? ""
        type = (try? c.decode(String.self, forKey: .type)) ?? ""
        username = (try? c.decode(String.self, forKey: .username)) ?? ""
        password = (try? c.decode(String.self, forKey: .password)) ?? ""
        notes = (try? c.decode(String.self, forKey: .notes)) ?? ""
        fields = (try? c.decode([Field].self, forKey: .fields)) ?? []
        favorite = (try? c.decode(Bool.self, forKey: .favorite)) ?? false
        let secret = (try? c.decode(String.self, forKey: .totp)) ?? ""
        otp = TOTP(
            secretOrURI: secret,
            algorithm: try? c.decode(String.self, forKey: .totpAlgorithm),
            digits: try? c.decode(Int.self, forKey: .totpDigits),
            period: try? c.decode(Int.self, forKey: .totpPeriod))
    }

    /// Reads + decodes the cached credentials from the shared App Group keychain.
    static func loadAll() -> [Credential] {
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

    // MARK: Matching

    var host: String? { Self.host(from: url) }

    var indexLetter: String {
        guard let first = name.trimmingCharacters(in: .whitespaces).uppercased().first,
              first.isLetter, first.isASCII else { return "#" }
        return String(first)
    }

    func matches(query q: String) -> Bool {
        name.lowercased().contains(q)
            || username.lowercased().contains(q)
            || url.lowercased().contains(q)
            || notes.lowercased().contains(q)
            || fields.contains { !$0.isSecret && $0.value.lowercased().contains(q) }
    }

    /// Same site: equal hosts or one a subdomain of the other
    /// ("accounts.hetzner.com" ↔ "hetzner.com"). Entries without a URL match
    /// when their name contains the site's main label ("Hetzner Online").
    func matches(hosts: [String]) -> Bool {
        guard !hosts.isEmpty else { return false }
        if let h = host {
            return hosts.contains { $0 == h || $0.hasSuffix("." + h) || h.hasSuffix("." + $0) }
        }
        let lowerName = name.lowercased()
        return hosts.contains { site in
            guard let label = Self.mainLabel(of: site), label.count >= 4 else { return false }
            return lowerName.contains(label)
        }
    }

    /// Extracts the host, tolerating scheme-less entries ("example.com") by
    /// retrying with an "https://" prefix; strips a leading "www.".
    static func host(from urlString: String) -> String? {
        let trimmed = urlString.trimmingCharacters(in: .whitespaces)
        guard !trimmed.isEmpty else { return nil }
        let raw = URL(string: trimmed)?.host ?? URL(string: "https://\(trimmed)")?.host
        guard var h = raw, !h.isEmpty else { return nil }
        if h.hasPrefix("www.") { h.removeFirst(4) }
        return h.lowercased()
    }

    /// "accounts.hetzner.com" → "hetzner" (second-to-last label).
    private static func mainLabel(of host: String) -> String? {
        let parts = host.split(separator: ".")
        guard parts.count >= 2 else { return parts.first.map(String.init) }
        return String(parts[parts.count - 2])
    }
}

// MARK: - TOTP (RFC 6238)

struct TOTP {
    enum Algorithm { case sha1, sha256, sha512 }

    let key: Data
    let algorithm: Algorithm
    let digits: Int
    let period: Int

    /// Accepts a bare base32 secret or an `otpauth://totp/…?secret=…` URI.
    /// Explicit parameters win over those in the URI; defaults are SHA1, 6, 30.
    init?(secretOrURI: String, algorithm: String?, digits: Int?, period: Int?) {
        let trimmed = secretOrURI.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return nil }
        var secret = trimmed
        var uriAlgo: String?, uriDigits: Int?, uriPeriod: Int?
        if trimmed.lowercased().hasPrefix("otpauth://"),
           let comps = URLComponents(string: trimmed) {
            let items = comps.queryItems ?? []
            func item(_ n: String) -> String? { items.first { $0.name.lowercased() == n }?.value }
            secret = item("secret") ?? ""
            uriAlgo = item("algorithm")
            uriDigits = item("digits").flatMap(Int.init)
            uriPeriod = item("period").flatMap(Int.init)
        }
        guard let key = Self.base32Decode(secret), !key.isEmpty else { return nil }
        self.key = key
        switch (algorithm ?? uriAlgo ?? "SHA1").uppercased().replacingOccurrences(of: "-", with: "") {
        case "SHA256": self.algorithm = .sha256
        case "SHA512": self.algorithm = .sha512
        default: self.algorithm = .sha1
        }
        let d = digits ?? uriDigits ?? 6
        self.digits = (6...10).contains(d) ? d : 6
        let p = period ?? uriPeriod ?? 30
        self.period = p > 0 ? p : 30
    }

    func code(at date: Date = Date()) -> String? {
        var counter = UInt64(date.timeIntervalSince1970 / Double(period)).bigEndian
        let counterData = Data(bytes: &counter, count: 8)
        let symmetric = SymmetricKey(data: key)
        let hash: Data
        switch algorithm {
        case .sha1: hash = Data(HMAC<Insecure.SHA1>.authenticationCode(for: counterData, using: symmetric))
        case .sha256: hash = Data(HMAC<SHA256>.authenticationCode(for: counterData, using: symmetric))
        case .sha512: hash = Data(HMAC<SHA512>.authenticationCode(for: counterData, using: symmetric))
        }
        let offset = Int(hash[hash.count - 1] & 0x0f)
        let binary = (UInt64(hash[offset] & 0x7f) << 24)
            | (UInt64(hash[offset + 1]) << 16)
            | (UInt64(hash[offset + 2]) << 8)
            | UInt64(hash[offset + 3])
        var modulus: UInt64 = 1
        for _ in 0..<digits { modulus *= 10 }
        let value = String(binary % modulus)
        return String(repeating: "0", count: max(0, digits - value.count)) + value
    }

    func secondsRemaining(at date: Date = Date()) -> Int {
        period - (Int(date.timeIntervalSince1970) % period)
    }

    /// RFC 4648 base32 decode (uppercase, no padding required), tolerant of
    /// lowercase, spaces and "=" padding as stored in entries.
    private static func base32Decode(_ input: String) -> Data? {
        let alphabet = Array("ABCDEFGHIJKLMNOPQRSTUVWXYZ234567")
        let clean = input.uppercased().filter { $0 != "=" && $0 != " " && $0 != "-" }
        guard !clean.isEmpty else { return nil }
        var bits = 0, value = 0
        var out = [UInt8]()
        for ch in clean {
            guard let idx = alphabet.firstIndex(of: ch) else { return nil }
            value = ((value << 5) | idx) & 0xFFFF
            bits += 5
            if bits >= 8 {
                bits -= 8
                out.append(UInt8((value >> bits) & 0xff))
            }
        }
        return Data(out)
    }
}
