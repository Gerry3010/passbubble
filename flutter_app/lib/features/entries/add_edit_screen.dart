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
import '../../shared/widgets/pb_button.dart';
import '../../shared/widgets/pb_text_field.dart';
import '../../shared/widgets/prompt_title.dart';
import 'entries_list_screen.dart' show entriesProvider, foldersProvider;

// ─── Custom field state ───────────────────────────────────────────────────────

class _CustomFieldState {
  final TextEditingController label;
  final TextEditingController value;
  CustomFieldType type;
  String? filename;
  String? mimeType;
  Uint8List? fileBytes;
  bool obscured;

  _CustomFieldState({
    String labelText = '',
    String valueText = '',
    this.type = CustomFieldType.text,
    this.filename,
    this.mimeType,
  })  : label = TextEditingController(text: labelText),
        value = TextEditingController(text: valueText),
        obscured = type == CustomFieldType.password || type == CustomFieldType.ssh;

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
        // Keep everything this form doesn't manage (see _extraData).
        const managed = {..._allFields, 'account_type', 'sign_in_with', 'custom_fields'};
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
    final cfList = <Map<String, dynamic>>[];
    for (final cf in _customFields) {
      final lbl = cf.label.text.trim();
      if (lbl.isEmpty) continue;
      if (cf.type == CustomFieldType.file) {
        if (cf.fileBytes != null) {
          cfList.add({
            'label': lbl,
            'value': base64Encode(cf.fileBytes!),
            'type': cf.type.apiValue,
            if (cf.filename != null) 'filename': cf.filename,
            if (cf.mimeType != null) 'mime_type': cf.mimeType,
          });
        } else if (cf.value.text.isNotEmpty) {
          // Existing file (loaded from vault, no new file picked)
          cfList.add({
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
        'label': lbl,
        'value': cf.value.text.trim(),
        'type': cf.type.apiValue,
      });
    }
    if (cfList.isNotEmpty) data['custom_fields'] = cfList;
    return data;
  }

  // ── Generate password ─────────────────────────────────────────────────────

  Future<void> _generatePassword() async {
    try {
      final resp = await ref.read(apiClientProvider).generate(length: 20, type: 'strong');
      if (resp.passwords.isNotEmpty) {
        setState(() => _ctrl['password']!.text = resp.passwords.first.password);
      }
    } catch (_) {}
  }

  // ── Save entry ────────────────────────────────────────────────────────────

  Future<void> _save() async {
    if (!_form.currentState!.validate()) return;
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
            onPressed: _generatePassword,
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
                  onPressed: _generatePassword,
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
      for (int i = 0; i < _customFields.length; i++) ...[
        const SizedBox(height: 8),
        _buildCustomField(i),
      ],
      const SizedBox(height: 8),
      TextButton.icon(
        icon: const Icon(Icons.add, size: 16),
        label: const Text('Add field'),
        style: TextButton.styleFrom(foregroundColor: AppTheme.green),
        onPressed: () => _showFieldTypePicker(),
      ),
    ],
  );

  Widget _buildCustomField(int i) {
    final cf = _customFields[i];
    final removeBtn = IconButton(
      icon: const Icon(Icons.remove_circle_outline, color: AppTheme.error, size: 20),
      onPressed: () { cf.dispose(); setState(() => _customFields.removeAt(i)); },
    );

    // Type badge chip
    final typeMeta = _cfTypes.firstWhere((t) => t.$1 == cf.type);
    final typeBadge = GestureDetector(
      onTap: () => _changeFieldType(i),
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
              onPressed: () => _pickFile(i),
            ),
          ]),
        ],
      );
    }

    final isObscured = cf.type == CustomFieldType.password || cf.type == CustomFieldType.ssh;
    final isMultiline = cf.type == CustomFieldType.note || cf.type == CustomFieldType.ssh;

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(children: [
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
              maxLines: isMultiline ? 4 : 1,
              suffixIcon: isObscured
                  ? IconButton(
                      icon: Icon(
                        cf.obscured ? Icons.visibility_off : Icons.visibility,
                        size: 18,
                        color: AppTheme.onBgDim,
                      ),
                      onPressed: () => setState(() => cf.obscured = !cf.obscured),
                    )
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

  void _changeFieldType(int i) {
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
      if (type == null) return;
      setState(() {
        _customFields[i].type = type;
        _customFields[i].obscured = type == CustomFieldType.password || type == CustomFieldType.ssh;
        if (type != CustomFieldType.file) {
          _customFields[i].fileBytes = null;
          _customFields[i].filename = null;
        }
      });
    });
  }

  Future<void> _pickFile(int i) async {
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
    setState(() {
      _customFields[i].fileBytes = Uint8List.fromList(bytes);
      _customFields[i].filename = file.name;
      _customFields[i].mimeType = file.extension != null
          ? 'application/${file.extension}'
          : 'application/octet-stream';
      if (_customFields[i].label.text.trim().isEmpty) {
        _customFields[i].label.text = file.name;
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
