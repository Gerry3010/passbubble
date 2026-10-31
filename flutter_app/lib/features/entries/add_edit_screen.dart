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

import 'dart:convert';
import 'dart:typed_data';

import 'package:file_picker/file_picker.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../core/api/api_client.dart';
import '../../core/api/models.dart';
import '../../core/auth/auth_service.dart';
import '../../core/crypto/vault_crypto.dart';
import '../../core/importexport/entry_record.dart' show CustomFieldType, CustomFieldTypeX;
import '../../core/theme/app_theme.dart';
import '../../core/util/entry_tags.dart';
import '../../shared/widgets/generator_dialog.dart';
import '../../shared/widgets/pb_button.dart';
import '../../shared/widgets/pb_text_field.dart';
import '../../shared/widgets/prompt_title.dart';
import 'entries_list_screen.dart'
    show entriesProvider, foldersProvider, searchIndexProvider;

// ─── Custom field state ───────────────────────────────────────────────────────

class _CustomFieldState {
  final TextEditingController label;
  final TextEditingController value;
  CustomFieldType type;
  String? filename;
  String? mimeType;
  Uint8List? fileBytes;
  bool obscured;

  /// Keys of the stored field map this form does not manage (written by other
  /// clients). Carried over verbatim on save, like the entry-level extra data.
  final Map<String, dynamic> extra;

  _CustomFieldState({
    String labelText = '',
    String valueText = '',
    this.type = CustomFieldType.text,
    this.filename,
    this.mimeType,
    this.extra = const {},
  })  : label = TextEditingController(text: labelText),
        value = TextEditingController(text: valueText),
        obscured = type == CustomFieldType.password || type == CustomFieldType.ssh;

  static const managedKeys = {'label', 'value', 'type', 'filename', 'mime_type'};

  void dispose() {
    label.dispose();
    value.dispose();
  }
}

// ─── Custom field type picker metadata ───────────────────────────────────────

const _cfTypes = [
  (CustomFieldType.text,     'Text',      Icons.text_fields),
  (CustomFieldType.password, 'Password',  Icons.lock_outline),
  (CustomFieldType.totp,     'TOTP',      Icons.schedule),
  (CustomFieldType.url,      'URL',       Icons.link),
  (CustomFieldType.email,    'Email',     Icons.email_outlined),
  (CustomFieldType.phone,    'Phone',     Icons.phone_outlined),
  (CustomFieldType.note,     'Note',      Icons.notes),
  (CustomFieldType.ssh,      'SSH Key',   Icons.terminal),
  (CustomFieldType.file,     'File',      Icons.attach_file),
];

// ─── Entry type metadata ──────────────────────────────────────────────────────

const _allTypes = [
  ('password',     'Password',     Icons.lock_outline),
  ('totp',         'TOTP',         Icons.schedule),
  ('note',         'Note',         Icons.notes),
  ('api-key',      'API Key',      Icons.vpn_key_outlined),
  ('ssh-key',      'SSH Key',      Icons.terminal),
  ('credit-card',  'Credit Card',  Icons.credit_card_outlined),
  ('bank-account', 'Bank Account', Icons.account_balance_outlined),
  ('identity',     'Identity',     Icons.badge_outlined),
  ('license',      'License',      Icons.confirmation_number_outlined),
];

class AddEditScreen extends ConsumerStatefulWidget {
  final String? editId;
  final String? folderId;

  /// Pre-selects the entry type on create (e.g. the wallet's "new card"
  /// shortcut passes 'credit-card'). Ignored on edit and for unknown types.
  final String? initialType;
  const AddEditScreen({super.key, this.editId, this.folderId, this.initialType});

  @override
  ConsumerState<AddEditScreen> createState() => _AddEditScreenState();
}

class _AddEditScreenState extends ConsumerState<AddEditScreen> {
  final _form = GlobalKey<FormState>();

  // All text controllers keyed by field name
  late final Map<String, TextEditingController> _ctrl;

  final List<_CustomFieldState> _customFields = [];

  // Tags in the user's order; the text field takes the next one.
  final List<String> _tags = [];
  final _tagCtrl = TextEditingController();
  final _tagFocus = FocusNode();
  String? _tagError;

  String _type = 'password';
  bool _loading = false;
  bool _obscurePassword = true;
  bool _obscureCvv = true;
  String? _error;

  String _accountType = 'checking';
  String _titleValue = '';
  String _signInWith = '';

  /// Decrypted keys this form does not manage (e.g. TOTP metadata written by
  /// other clients). Carried over verbatim on save so an edit here never
  /// silently drops them.
  Map<String, dynamic> _extraData = {};

  /// The folder the entry lives in. On create this seeds from the folder the
  /// user is currently browsing; on edit it is loaded from the entry and can be
  /// changed via the folder picker to move the entry.
  String? _selectedFolderId;

  bool get isEdit => widget.editId != null;

  @override
  void initState() {
    super.initState();
    _ctrl = {
      for (final key in _allFields) key: TextEditingController(),
    };
    _selectedFolderId = widget.folderId;
    if (!isEdit && _allTypes.any((t) => t.$1 == widget.initialType)) {
      _type = widget.initialType!;
    }
    if (isEdit) _loadEntry();
  }

  static const _allFields = [
    'name', 'url', 'username', 'password', 'notes', 'totp_secret',
    'card_number', 'holder_name', 'expiry_month', 'expiry_year', 'cvv',
    'bank_name', 'iban', 'bic', 'account_number',
    'title', 'first_name', 'last_name', 'company', 'email', 'phone',
    'street', 'city', 'state', 'postal_code', 'country',
    'product_name', 'license_key', 'purchase_email', 'purchase_date', 'expires_at',
  ];

  @override
  void dispose() {
    for (final c in _ctrl.values) { c.dispose(); }
    for (final cf in _customFields) { cf.dispose(); }
    _tagCtrl.dispose();
    _tagFocus.dispose();
    super.dispose();
  }

  // ── Load existing entry ───────────────────────────────────────────────────

  Future<void> _loadEntry() async {
    setState(() => _loading = true);
    try {
      final entry = await ref.read(apiClientProvider).getEntry(widget.editId!);
      _ctrl['name']!.text = entry.name;
      _ctrl['url']!.text = entry.url;
      setState(() {
        _type = entry.type;
        _selectedFolderId = entry.folderId;
      });

      final authSvc = ref.read(authServiceProvider);
      if (authSvc.privX25519 != null && entry.entryKey != null) {
        final dataKey = await VaultCrypto.decryptDataKey(
          entry.entryKey!.encryptedKey,
          authSvc.privX25519!,
          authSvc.privMLKEM!,
        );
        final data = await VaultCrypto.decryptEntryData(
          entry.encryptedData,
          Uint8List.fromList(dataKey),
        );
        // Fill all controllers from decrypted data
        for (final key in _allFields) {
          if (data.containsKey(key)) {
            _ctrl[key]!.text = (data[key] as String?) ?? '';
          }
        }
        if (data.containsKey('account_type')) {
          setState(() => _accountType = (data['account_type'] as String?) ?? 'checking');
        }
        if (data.containsKey('title')) {
          setState(() => _titleValue = (data['title'] as String?) ?? '');
        }
        if (data.containsKey('sign_in_with')) {
          setState(() => _signInWith = (data['sign_in_with'] as String?) ?? '');
        }
        _tags.addAll(tagsOf(data));
        // Keep everything this form doesn't manage (see _extraData).
        const managed = {
          ..._allFields, 'account_type', 'sign_in_with', 'custom_fields', tagsKey,
        };
        _extraData = {
          for (final e in data.entries)
            if (!managed.contains(e.key)) e.key: e.value,
        };
        // Custom fields
        final raw = data['custom_fields'];
        if (raw is List) {
          for (final cf in raw) {
            if (cf is Map<String, dynamic>) {
              final cfType = CustomFieldTypeX.fromApi(cf['type'] as String?);
              _customFields.add(_CustomFieldState(
                labelText: cf['label'] as String? ?? '',
                valueText: cf['value'] as String? ?? '',
                type: cfType,
                filename: cf['filename'] as String?,
                mimeType: cf['mime_type'] as String?,
                extra: {
                  for (final e in cf.entries)
                    if (!_CustomFieldState.managedKeys.contains(e.key)) e.key: e.value,
                },
              ));
            }
          }
        }
      }
    } catch (e) {
      setState(() => _error = e.toString());
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  // ── Collect all field values into encrypted data map ─────────────────────

  Map<String, dynamic> _collectData() {
    final Map<String, dynamic> data = Map.of(_extraData);
    for (final key in _allFields) {
      final val = _ctrl[key]!.text.trim();
      if (val.isNotEmpty) data[key] = val;
    }
    if (_type == 'bank-account' && _accountType.isNotEmpty) {
      data['account_type'] = _accountType;
    }
    if (_type == 'password' && _signInWith.isNotEmpty) {
      data['sign_in_with'] = _signInWith;
    }
    // The list order is the user's (drag & drop) order; other clients render
    // custom fields in stored order, so it syncs as-is.
    final cfList = <Map<String, dynamic>>[];
    for (final cf in _customFields) {
      final lbl = cf.label.text.trim();
      if (lbl.isEmpty) continue;
      if (cf.type == CustomFieldType.file) {
        if (cf.fileBytes != null) {
          cfList.add({
            ...cf.extra,
            'label': lbl,
            'value': base64Encode(cf.fileBytes!),
            'type': cf.type.apiValue,
            if (cf.filename != null) 'filename': cf.filename,
            if (cf.mimeType != null) 'mime_type': cf.mimeType,
          });
        } else if (cf.value.text.isNotEmpty) {
          // Existing file (loaded from vault, no new file picked)
          cfList.add({
            ...cf.extra,
            'label': lbl,
            'value': cf.value.text,
            'type': cf.type.apiValue,
            if (cf.filename != null) 'filename': cf.filename,
            if (cf.mimeType != null) 'mime_type': cf.mimeType,
          });
        }
        continue;
      }
      cfList.add({
        ...cf.extra,
        'label': lbl,
        'value': cf.value.text.trim(),
        'type': cf.type.apiValue,
      });
    }
    if (cfList.isNotEmpty) data['custom_fields'] = cfList;
    putTags(data, _tags);
    return data;
  }

  // ── Generate password ─────────────────────────────────────────────────────

  /// Opens the generator dialog; only OK writes the password into [target].
  Future<void> _generateInto(TextEditingController target) async {
    final password = await showGeneratorDialog(context, ref);
    if (password != null && mounted) setState(() => target.text = password);
  }

  // ── Save entry ────────────────────────────────────────────────────────────

  Future<void> _save() async {
    if (!_form.currentState!.validate()) return;
    // Text still in the tag field counts as a tag, unless it is refused.
    if (!_commitTagInput()) return;
    final authSvc = ref.read(authServiceProvider);
    if (authSvc.privX25519 == null) {
      setState(() => _error = 'Vault is locked');
      return;
    }
    setState(() { _loading = true; _error = null; });

    try {
      final (:encryptedData, :dataNonce, :dataKey) =
          await VaultCrypto.encryptEntryData(_collectData());

      final myPubX25519 = await authSvc.getPubX25519();
      final myPubMlkem = await authSvc.getPubMlkem768();
      if (myPubX25519 == null || myPubMlkem == null) {
        throw Exception('Public key not found');
      }
      final myUserId = await authSvc.getUserId();
      final encKey = await VaultCrypto.encryptDataKey(dataKey, myPubX25519, myPubMlkem);

      if (isEdit) {
        await ref.read(apiClientProvider).updateEntry(
          widget.editId!,
          UpdateEntryRequest(
            folderId: _selectedFolderId,
            name: _ctrl['name']!.text.trim(),
            url: _ctrl['url']!.text.trim(),
            encryptedData: encryptedData,
            dataNonce: dataNonce,
            entryKeys: [EntryKey(userId: myUserId ?? '', encryptedKey: encKey)],
          ),
        );
      } else {
        await ref.read(apiClientProvider).createEntry(
          CreateEntryRequest(
            type: _type,
            name: _ctrl['name']!.text.trim(),
            url: _ctrl['url']!.text.trim(),
            encryptedData: encryptedData,
            dataNonce: dataNonce,
            entryKeys: [EntryKey(userId: myUserId ?? '', encryptedKey: encKey)],
            folderId: _selectedFolderId,
          ),
        );
      }
      ref.invalidate(entriesProvider);
      authSvc.refreshAutofill().ignore(); // refresh the system autofill cache
      if (mounted) context.go('/entries');
    } catch (e) {
      setState(() => _error = e.toString());
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  // ── Folder picker (move entry between folders) ────────────────────────────

  Widget _folderPicker() {
    final foldersAsync = ref.watch(foldersProvider);
    return foldersAsync.maybeWhen(
      data: (roots) {
        // Flatten the folder tree into indented dropdown options.
        final options = <({String? id, String label})>[
          (id: null, label: '(No folder)'),
        ];
        void walk(List<FolderResponse> fs, int depth) {
          for (final f in fs) {
            options.add((id: f.id, label: '${'    ' * depth}${f.name}'));
            walk(f.children, depth + 1);
          }
        }
        walk(roots, 0);

        final ids = options.map((o) => o.id).toSet();
        final value = ids.contains(_selectedFolderId) ? _selectedFolderId : null;
        return DropdownButtonFormField<String?>(
          key: ValueKey(value),
          initialValue: value,
          isExpanded: true,
          decoration: const InputDecoration(
            labelText: 'Folder',
            prefixIcon: Icon(Icons.folder_outlined),
            border: OutlineInputBorder(),
          ),
          items: [
            for (final o in options)
              DropdownMenuItem<String?>(
                value: o.id,
                child: Text(o.label, overflow: TextOverflow.ellipsis),
              ),
          ],
          onChanged: (v) => setState(() => _selectedFolderId = v),
        );
      },
      orElse: () => const SizedBox.shrink(),
    );
  }

  // ── Tags ──────────────────────────────────────────────────────────────────

  /// Every tag in the vault (from the decrypted list index).
  List<String> _vaultTags() =>
      ref.read(searchIndexProvider).valueOrNull?.allTags ?? const [];

  /// Adds [raw] as a tag unless it is blank or already on the entry. A tag
  /// the vault already has keeps the vault's spelling. Returns false when it
  /// is too long.
  bool _addTag(String raw) {
    if (tagInputError(raw) != null) return false;
    final tag = cleanTag(raw);
    if (tag.isEmpty || containsTag(_tags, tag)) return true;
    _tags.add(_vaultTags().firstWhere((t) => sameTag(t, tag), orElse: () => tag));
    return true;
  }

  /// Turns [raw] (default: the typed text) into a tag — on Enter, on picking
  /// a suggestion and on save. Returns false, keeping the text and showing
  /// why, when it is refused.
  bool _commitTagInput([String? raw]) {
    final text = raw ?? _tagCtrl.text;
    final ok = _addTag(text);
    setState(() {
      if (ok) _tagCtrl.clear();
      _tagError = ok ? null : tagInputError(text);
    });
    return ok;
  }

  /// A comma ends a tag, so "a, b" typed or pasted becomes two tags; a part
  /// that is too long stays in the field with the error.
  void _onTagInputChanged(String value) {
    if (value.contains(',')) {
      final parts = value.split(',');
      final kept = <String>[];
      for (final part in parts.take(parts.length - 1)) {
        if (!_addTag(part)) kept.add(part.trim());
      }
      final rest = parts.last.trimLeft();
      if (rest.isNotEmpty) kept.add(rest);
      final text = kept.join(', ');
      _tagCtrl.value = TextEditingValue(
          text: text, selection: TextSelection.collapsed(offset: text.length));
    }
    // Also refreshes the suggestions for the new text.
    setState(() => _tagError = tagInputError(_tagCtrl.text));
  }

  void _removeTag(String tag) => setState(() => _tags.remove(tag));

  Widget _tagsInput() {
    final vaultTags =
        ref.watch(searchIndexProvider).valueOrNull?.allTags ?? const <String>[];
    final suggestions = suggestTags(vaultTags, _tagCtrl.text, exclude: _tags);
    return Padding(
      padding: const EdgeInsets.only(top: 12),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          if (_tags.isNotEmpty)
            Padding(
              padding: const EdgeInsets.only(bottom: 8),
              child: Wrap(
                spacing: 8,
                runSpacing: 4,
                children: [
                  for (final tag in _tags)
                    InputChip(
                      avatar: const Icon(Icons.tag, size: 14, color: AppTheme.green),
                      label: Text(tag),
                      onDeleted: () => _removeTag(tag),
                      deleteIcon: const Icon(Icons.close, size: 16),
                      deleteButtonTooltipMessage: 'Remove tag',
                    ),
                ],
              ),
            ),
          TextField(
            key: const ValueKey('tag-input'),
            controller: _tagCtrl,
            focusNode: _tagFocus,
            textInputAction: TextInputAction.done,
            onChanged: _onTagInputChanged,
            onSubmitted: (_) => _commitTagInput(),
            // Keep the focus (and the keyboard) for the next tag.
            onEditingComplete: () {},
            decoration: InputDecoration(
              labelText: 'Tags',
              hintText: 'Add a tag, then Enter or comma',
              prefixIcon: const Icon(Icons.sell_outlined),
              errorText: _tagError,
            ),
          ),
          if (suggestions.isNotEmpty)
            Padding(
              padding: const EdgeInsets.only(top: 6),
              child: Wrap(
                spacing: 8,
                runSpacing: 4,
                children: [
                  for (final s in suggestions)
                    ActionChip(
                      avatar: const Icon(Icons.add, size: 14, color: AppTheme.green),
                      label: Text(s),
                      tooltip: 'Add tag $s',
                      onPressed: () {
                        _commitTagInput(s);
                        _tagFocus.requestFocus();
                      },
                    ),
                ],
              ),
            ),
        ],
      ),
    );
  }

  // ── Build type-specific form fields ───────────────────────────────────────

  List<Widget> _buildTypeFields() {
    return switch (_type) {
      'password' => _passwordFields(),
      'totp'     => _totpFields(),
      'note'     => _noteFields(),
      'api-key'  => _apiKeyFields(),
      'ssh-key'  => _sshKeyFields(),
      'credit-card'  => _creditCardFields(),
      'bank-account' => _bankAccountFields(),
      'identity'     => _identityFields(),
      'license'      => _licenseFields(),
      _              => _passwordFields(),
    };
  }

  static const _ssoProviders = [
    ('google', 'Google'),
    ('apple', 'Apple'),
    ('microsoft', 'Microsoft'),
    ('github', 'GitHub'),
    ('facebook', 'Facebook'),
  ];

  List<Widget> _passwordFields() => [
    _field('username', 'Username', Icons.person_outline),
    _passwordField(),
    _field('totp_secret', 'TOTP Secret (optional, base32)', Icons.schedule),
    Padding(
      padding: const EdgeInsets.only(top: 4),
      child: DropdownButtonFormField<String>(
        key: ValueKey(_signInWith),
        initialValue: _signInWith,
        decoration: const InputDecoration(
          labelText: 'Sign in with (SSO)',
          prefixIcon: Icon(Icons.login_outlined),
          border: OutlineInputBorder(),
        ),
        items: [
          const DropdownMenuItem(value: '', child: Text('—')),
          for (final (value, label) in _ssoProviders)
            DropdownMenuItem(value: value, child: Text(label)),
        ],
        onChanged: (v) => setState(() => _signInWith = v ?? ''),
      ),
    ),
    _field('notes', 'Notes', Icons.notes, maxLines: 3),
  ];

  List<Widget> _totpFields() => [
    _field('username', 'Account / Username', Icons.person_outline),
    _field('totp_secret', 'TOTP Secret (base32)', Icons.schedule,
        validator: (v) => v!.isEmpty ? 'Required' : null),
    _field('notes', 'Notes', Icons.notes, maxLines: 3),
  ];

  List<Widget> _noteFields() => [
    _field('notes', 'Content', Icons.notes, maxLines: 8,
        validator: (v) => v!.isEmpty ? 'Required' : null),
  ];

  List<Widget> _apiKeyFields() => [
    _field('username', 'Key ID / Client ID', Icons.person_outline),
    _obscuredField('password', 'API Key / Secret', Icons.vpn_key_outlined,
        ctrl: 'password', obscure: _obscurePassword,
        onToggle: () => setState(() => _obscurePassword = !_obscurePassword),
        showGenerate: false),
    _field('notes', 'Notes', Icons.notes, maxLines: 3),
  ];

  List<Widget> _sshKeyFields() => [
    _field('username', 'User', Icons.person_outline),
    _field('password', 'Private Key', Icons.terminal, maxLines: 6,
        validator: (v) => v!.isEmpty ? 'Required' : null),
    _field('notes', 'Notes / Passphrase', Icons.notes, maxLines: 3),
  ];

  List<Widget> _creditCardFields() => [
    _field('card_number', 'Card Number', Icons.credit_card_outlined,
        keyboardType: TextInputType.number,
        validator: (v) => v!.isEmpty ? 'Required' : null),
    _field('holder_name', 'Cardholder Name', Icons.badge_outlined),
    Row(children: [
      Expanded(child: _field('expiry_month', 'MM', Icons.calendar_today,
          keyboardType: TextInputType.number)),
      const SizedBox(width: 12),
      Expanded(child: _field('expiry_year', 'YYYY', Icons.calendar_today,
          keyboardType: TextInputType.number)),
    ]),
    _obscuredField('cvv', 'CVV', Icons.lock_outline,
        ctrl: 'cvv', obscure: _obscureCvv,
        onToggle: () => setState(() => _obscureCvv = !_obscureCvv),
        showGenerate: false),
    _field('notes', 'Notes', Icons.notes, maxLines: 2),
  ];

  List<Widget> _bankAccountFields() => [
    _field('bank_name', 'Bank Name', Icons.account_balance_outlined),
    _field('iban', 'IBAN', Icons.numbers,
        keyboardType: TextInputType.text),
    _field('bic', 'BIC / SWIFT', Icons.code),
    _field('account_number', 'Account Number', Icons.numbers),
    Padding(
      padding: const EdgeInsets.only(top: 4),
      child: DropdownButtonFormField<String>(
        key: ValueKey(_accountType),
        initialValue: _accountType,
        decoration: InputDecoration(
          labelText: 'Account Type',
          prefixIcon: const Icon(Icons.swap_horiz),
          border: const OutlineInputBorder(),
        ),
        items: const [
          DropdownMenuItem(value: 'checking', child: Text('Checking')),
          DropdownMenuItem(value: 'savings', child: Text('Savings')),
        ],
        onChanged: (v) => setState(() => _accountType = v ?? 'checking'),
      ),
    ),
    _field('notes', 'Notes', Icons.notes, maxLines: 2),
  ];

  List<Widget> _identityFields() => [
    Row(children: [
      SizedBox(
        width: 100,
        child: DropdownButtonFormField<String>(
          key: ValueKey(_titleValue),
          initialValue: _titleValue,
          decoration: const InputDecoration(
            labelText: 'Title',
            border: OutlineInputBorder(),
            contentPadding: EdgeInsets.symmetric(horizontal: 12, vertical: 14),
          ),
          items: const [
            DropdownMenuItem(value: '', child: Text('—')),
            DropdownMenuItem(value: 'Mr', child: Text('Mr')),
            DropdownMenuItem(value: 'Ms', child: Text('Ms')),
            DropdownMenuItem(value: 'Mrs', child: Text('Mrs')),
            DropdownMenuItem(value: 'Dr', child: Text('Dr')),
          ],
          onChanged: (v) => setState(() {
            _titleValue = v ?? '';
            _ctrl['title']!.text = _titleValue;
          }),
        ),
      ),
      const SizedBox(width: 12),
      Expanded(child: _field('first_name', 'First Name', Icons.person_outline)),
    ]),
    _field('last_name', 'Last Name', Icons.person_outline),
    _field('company', 'Company', Icons.business_outlined),
    _field('email', 'Email', Icons.email_outlined,
        keyboardType: TextInputType.emailAddress),
    _field('phone', 'Phone', Icons.phone_outlined,
        keyboardType: TextInputType.phone),
    const Padding(
      padding: EdgeInsets.symmetric(vertical: 8),
      child: Text('ADDRESS', style: TextStyle(color: AppTheme.green, fontSize: 11, letterSpacing: 2)),
    ),
    _field('street', 'Street', Icons.home_outlined),
    Row(children: [
      Expanded(flex: 2, child: _field('city', 'City', Icons.location_city)),
      const SizedBox(width: 8),
      Expanded(child: _field('state', 'State', Icons.map_outlined)),
    ]),
    Row(children: [
      Expanded(child: _field('postal_code', 'Postal Code', Icons.markunread_mailbox_outlined)),
      const SizedBox(width: 8),
      Expanded(flex: 2, child: _field('country', 'Country', Icons.flag_outlined)),
    ]),
    _field('notes', 'Notes', Icons.notes, maxLines: 2),
  ];

  List<Widget> _licenseFields() => [
    _field('product_name', 'Product Name', Icons.apps_outlined,
        validator: (v) => v!.isEmpty ? 'Required' : null),
    _field('license_key', 'License Key', Icons.confirmation_number_outlined,
        validator: (v) => v!.isEmpty ? 'Required' : null),
    _field('purchase_email', 'Purchase Email', Icons.email_outlined,
        keyboardType: TextInputType.emailAddress),
    _field('purchase_date', 'Purchase Date (YYYY-MM-DD)', Icons.calendar_today),
    _field('expires_at', 'Expires At (YYYY-MM-DD)', Icons.event_outlined),
    _field('notes', 'Notes', Icons.notes, maxLines: 2),
  ];

  // ── Field helpers ─────────────────────────────────────────────────────────

  Widget _field(
    String key,
    String label,
    IconData icon, {
    int maxLines = 1,
    TextInputType? keyboardType,
    String? Function(String?)? validator,
  }) =>
      Padding(
        padding: const EdgeInsets.only(top: 12),
        child: PbTextField(
          label: label,
          controller: _ctrl[key]!,
          prefixIcon: icon,
          maxLines: maxLines,
          keyboardType: keyboardType,
          validator: validator,
        ),
      );

  Widget _passwordField() => Padding(
    padding: const EdgeInsets.only(top: 12),
    child: PbTextField(
      label: 'Password',
      controller: _ctrl['password']!,
      obscureText: _obscurePassword,
      prefixIcon: Icons.lock_outline,
      suffixIcon: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          IconButton(
            icon: Icon(
              _obscurePassword ? Icons.visibility_off : Icons.visibility,
              size: 18,
              color: AppTheme.onBgDim,
            ),
            onPressed: () => setState(() => _obscurePassword = !_obscurePassword),
          ),
          IconButton(
            icon: const Icon(Icons.auto_fix_high, size: 18, color: AppTheme.onBgDim),
            tooltip: 'Generate',
            onPressed: () => _generateInto(_ctrl['password']!),
          ),
        ],
      ),
      validator: (v) => v!.isEmpty ? 'Required' : null,
    ),
  );

  Widget _obscuredField(
    String label,
    String displayLabel,
    IconData icon, {
    required String ctrl,
    required bool obscure,
    required VoidCallback onToggle,
    bool showGenerate = false,
  }) =>
      Padding(
        padding: const EdgeInsets.only(top: 12),
        child: PbTextField(
          label: displayLabel,
          controller: _ctrl[ctrl]!,
          obscureText: obscure,
          prefixIcon: icon,
          suffixIcon: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              IconButton(
                icon: Icon(
                  obscure ? Icons.visibility_off : Icons.visibility,
                  size: 18,
                  color: AppTheme.onBgDim,
                ),
                onPressed: onToggle,
              ),
              if (showGenerate)
                IconButton(
                  icon: const Icon(Icons.auto_fix_high, size: 18, color: AppTheme.onBgDim),
                  tooltip: 'Generate',
                  onPressed: () => _generateInto(_ctrl[ctrl]!),
                ),
            ],
          ),
        ),
      );

  // ── Custom fields section ─────────────────────────────────────────────────

  Widget _buildCustomFields() => Column(
    crossAxisAlignment: CrossAxisAlignment.start,
    children: [
      const Padding(
        padding: EdgeInsets.only(top: 20, bottom: 4),
        child: Text('CUSTOM FIELDS',
            style: TextStyle(color: AppTheme.green, fontSize: 11, letterSpacing: 2)),
      ),
      // Reordered by drag & drop on each field's handle only (no default
      // handles / long-press on the whole row), so the text fields stay fully
      // usable. Embedded in the form's scroll view, hence no own scrolling.
      ReorderableListView(
        shrinkWrap: true,
        physics: const NeverScrollableScrollPhysics(),
        padding: EdgeInsets.zero,
        buildDefaultDragHandles: false,
        onReorderItem: _moveCustomField,
        children: [
          for (int i = 0; i < _customFields.length; i++)
            Padding(
              key: ObjectKey(_customFields[i]),
              padding: const EdgeInsets.only(top: 8),
              child: _buildCustomField(i),
            ),
        ],
      ),
      const SizedBox(height: 8),
      TextButton.icon(
        icon: const Icon(Icons.add, size: 16),
        label: const Text('Add field'),
        style: TextButton.styleFrom(foregroundColor: AppTheme.green),
        onPressed: () => _showFieldTypePicker(),
      ),
    ],
  );

  /// Moves a custom field to a new position. The list order is what gets
  /// saved, so the new order syncs to every client.
  void _moveCustomField(int oldIndex, int newIndex) {
    setState(() => _customFields.insert(newIndex, _customFields.removeAt(oldIndex)));
  }

  void _removeCustomField(_CustomFieldState cf) {
    setState(() => _customFields.remove(cf));
    // Dispose once the field's widgets are gone, not while they still listen.
    WidgetsBinding.instance.addPostFrameCallback((_) => cf.dispose());
  }

  Widget _buildCustomField(int i) {
    final cf = _customFields[i];
    final removeBtn = IconButton(
      icon: const Icon(Icons.remove_circle_outline, color: AppTheme.error, size: 20),
      onPressed: () => _removeCustomField(cf),
    );
    final dragHandle = ReorderableDragStartListener(
      index: i,
      child: const MouseRegion(
        cursor: SystemMouseCursors.grab,
        child: Padding(
          padding: EdgeInsets.only(right: 4),
          child: Icon(Icons.drag_indicator,
              color: AppTheme.onBgDim, semanticLabel: 'Reorder field'),
        ),
      ),
    );

    // Type badge chip
    final typeMeta = _cfTypes.firstWhere((t) => t.$1 == cf.type);
    final typeBadge = GestureDetector(
      onTap: () => _changeFieldType(cf),
      child: Chip(
        avatar: Icon(typeMeta.$3, size: 14, color: AppTheme.green),
        label: Text(typeMeta.$2, style: const TextStyle(fontSize: 11, color: AppTheme.green)),
        side: const BorderSide(color: AppTheme.green),
        backgroundColor: AppTheme.greenFaint,
        padding: EdgeInsets.zero,
        materialTapTargetSize: MaterialTapTargetSize.shrinkWrap,
      ),
    );

    if (cf.type == CustomFieldType.file) {
      return Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(children: [
            dragHandle,
            Expanded(
              child: PbTextField(
                label: 'Label',
                controller: cf.label,
                prefixIcon: Icons.label_outline,
              ),
            ),
            const SizedBox(width: 8),
            removeBtn,
          ]),
          const SizedBox(height: 6),
          Row(children: [
            typeBadge,
            const SizedBox(width: 8),
            Expanded(
              child: cf.fileBytes != null || cf.filename != null
                  ? Row(children: [
                      const Icon(Icons.insert_drive_file_outlined, size: 16, color: AppTheme.onBgDim),
                      const SizedBox(width: 6),
                      Expanded(
                        child: Text(
                          cf.filename ?? 'file',
                          overflow: TextOverflow.ellipsis,
                          style: const TextStyle(fontSize: 13),
                        ),
                      ),
                    ])
                  : const Text('No file selected', style: TextStyle(color: AppTheme.onBgDim, fontSize: 13)),
            ),
            TextButton.icon(
              icon: const Icon(Icons.attach_file, size: 16),
              label: const Text('Pick file'),
              style: TextButton.styleFrom(foregroundColor: AppTheme.green),
              onPressed: () => _pickFile(cf),
            ),
          ]),
        ],
      );
    }

    final isObscured = cf.type == CustomFieldType.password || cf.type == CustomFieldType.ssh;
    final isMultiline = cf.type == CustomFieldType.note || cf.type == CustomFieldType.ssh;
    final revealBtn = IconButton(
      icon: Icon(
        cf.obscured ? Icons.visibility_off : Icons.visibility,
        size: 18,
        color: AppTheme.onBgDim,
      ),
      onPressed: () => setState(() => cf.obscured = !cf.obscured),
    );

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(children: [
          dragHandle,
          Expanded(
            child: PbTextField(
              label: 'Label',
              controller: cf.label,
              prefixIcon: Icons.label_outline,
            ),
          ),
          const SizedBox(width: 8),
          removeBtn,
        ]),
        const SizedBox(height: 6),
        Row(children: [
          typeBadge,
          const SizedBox(width: 8),
          Expanded(
            child: PbTextField(
              label: 'Value',
              controller: cf.value,
              prefixIcon: Icons.edit_outlined,
              obscureText: isObscured ? cf.obscured : false,
              // A masked field must be single-line (TextField asserts it),
              // so a hidden SSH key grows to 4 lines only once revealed.
              maxLines: isMultiline && !(isObscured && cf.obscured) ? 4 : 1,
              // Passwords get a generator next to the eye; SSH keys don't.
              suffixIcon: cf.type == CustomFieldType.password
                  ? Row(
                      mainAxisSize: MainAxisSize.min,
                      children: [
                        revealBtn,
                        IconButton(
                          icon: const Icon(Icons.auto_fix_high, size: 18, color: AppTheme.onBgDim),
                          tooltip: 'Generate',
                          onPressed: () => _generateInto(cf.value),
                        ),
                      ],
                    )
                  : isObscured
                      ? revealBtn
                      : null,
            ),
          ),
        ]),
      ],
    );
  }

  void _showFieldTypePicker() {
    showModalBottomSheet<CustomFieldType>(
      context: context,
      builder: (ctx) => SafeArea(
        child: Padding(
          padding: const EdgeInsets.all(16),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              const Text('Add field',
                  style: TextStyle(fontSize: 16, fontWeight: FontWeight.w600)),
              const SizedBox(height: 12),
              Wrap(
                spacing: 8,
                runSpacing: 8,
                children: [
                  for (final (type, label, icon) in _cfTypes)
                    ActionChip(
                      avatar: Icon(icon, size: 16),
                      label: Text(label),
                      onPressed: () => Navigator.pop(ctx, type),
                    ),
                ],
              ),
            ],
          ),
        ),
      ),
    ).then((type) {
      if (type == null) return;
      setState(() => _customFields.add(_CustomFieldState(type: type)));
    });
  }

  void _changeFieldType(_CustomFieldState cf) {
    showModalBottomSheet<CustomFieldType>(
      context: context,
      builder: (ctx) => SafeArea(
        child: Padding(
          padding: const EdgeInsets.all(16),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              const Text('Change field type',
                  style: TextStyle(fontSize: 16, fontWeight: FontWeight.w600)),
              const SizedBox(height: 12),
              Wrap(
                spacing: 8,
                runSpacing: 8,
                children: [
                  for (final (type, label, icon) in _cfTypes)
                    ActionChip(
                      avatar: Icon(icon, size: 16),
                      label: Text(label),
                      onPressed: () => Navigator.pop(ctx, type),
                    ),
                ],
              ),
            ],
          ),
        ),
      ),
    ).then((type) {
      if (type == null || !_customFields.contains(cf)) return;
      setState(() {
        cf.type = type;
        cf.obscured = type == CustomFieldType.password || type == CustomFieldType.ssh;
        if (type != CustomFieldType.file) {
          cf.fileBytes = null;
          cf.filename = null;
        }
      });
    });
  }

  Future<void> _pickFile(_CustomFieldState cf) async {
    final result = await FilePicker.platform.pickFiles(withData: true);
    if (result == null || result.files.isEmpty) return;
    final file = result.files.first;
    final bytes = file.bytes;
    if (bytes == null) return;

    const maxBytes = 10 * 1024 * 1024; // 10 MB
    if (bytes.length > maxBytes) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(content: Text('File exceeds the 10 MB limit')),
        );
      }
      return;
    }
    if (!mounted || !_customFields.contains(cf)) return;
    setState(() {
      cf.fileBytes = Uint8List.fromList(bytes);
      cf.filename = file.name;
      cf.mimeType = file.extension != null
          ? 'application/${file.extension}'
          : 'application/octet-stream';
      if (cf.label.text.trim().isEmpty) {
        cf.label.text = file.name;
      }
    });
  }

  // ── Build ─────────────────────────────────────────────────────────────────

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: PromptTitle(isEdit ? 'edit entry' : 'new entry')),
      body: SingleChildScrollView(
        padding: const EdgeInsets.all(16),
        child: Form(
          key: _form,
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              // Type selector
              Wrap(
                spacing: 8,
                runSpacing: 4,
                children: [
                  for (final (type, label, _) in _allTypes)
                    ChoiceChip(
                      label: Text(label),
                      selected: _type == type,
                      onSelected: isEdit ? null : (_) => setState(() => _type = type),
                      selectedColor: AppTheme.greenFaint,
                      side: BorderSide(
                        color: _type == type ? AppTheme.green : AppTheme.border,
                      ),
                    ),
                ],
              ),
              const SizedBox(height: 16),

              // Name (always shown)
              PbTextField(
                label: 'Name *',
                controller: _ctrl['name']!,
                prefixIcon: Icons.label_outline,
                validator: (v) => v!.isEmpty ? 'Required' : null,
              ),

              // URL (shown for most types)
              if (_type != 'identity' && _type != 'bank-account') ...[
                const SizedBox(height: 12),
                PbTextField(
                  label: 'URL',
                  controller: _ctrl['url']!,
                  keyboardType: TextInputType.url,
                  prefixIcon: Icons.link,
                ),
              ],

              // Folder — pick where the entry lives (and move it between folders)
              const SizedBox(height: 12),
              _folderPicker(),

              // Tags — free-form labels, filterable in the list
              _tagsInput(),

              // Type-specific fields
              ..._buildTypeFields(),

              // Custom fields (available for all types)
              _buildCustomFields(),

              // Error
              if (_error != null) ...[
                const SizedBox(height: 12),
                Container(
                  padding: const EdgeInsets.all(12),
                  decoration: BoxDecoration(border: Border.all(color: AppTheme.error)),
                  child: Text(_error!, style: const TextStyle(color: AppTheme.error)),
                ),
              ],

              const SizedBox(height: 24),
              SizedBox(
                width: double.infinity,
                child: PbButton(
                  label: isEdit ? 'Save Changes' : 'Save Entry',
                  onPressed: _loading ? null : _save,
                  loading: _loading,
                  icon: Icons.save_outlined,
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
