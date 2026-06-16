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

import 'dart:typed_data';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../core/api/api_client.dart';
import '../../core/api/models.dart';
import '../../core/auth/auth_service.dart';
import '../../core/crypto/vault_crypto.dart';
import '../../core/theme/app_theme.dart';
import '../../shared/widgets/pb_button.dart';
import '../../shared/widgets/pb_text_field.dart';

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
  const AddEditScreen({super.key, this.editId});

  @override
  ConsumerState<AddEditScreen> createState() => _AddEditScreenState();
}

class _AddEditScreenState extends ConsumerState<AddEditScreen> {
  final _form = GlobalKey<FormState>();

  // All text controllers keyed by field name
  late final Map<String, TextEditingController> _ctrl;

  // Custom fields: list of (label, value) controller pairs
  final List<(TextEditingController, TextEditingController)> _customFields = [];

  String _type = 'password';
  bool _loading = false;
  bool _obscurePassword = true;
  bool _obscureCvv = true;
  String? _error;

  String _accountType = 'checking';
  String _titleValue = '';

  bool get isEdit => widget.editId != null;

  @override
  void initState() {
    super.initState();
    _ctrl = {
      for (final key in _allFields) key: TextEditingController(),
    };
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
    for (final (l, v) in _customFields) { l.dispose(); v.dispose(); }
    super.dispose();
  }

  // ── Load existing entry ───────────────────────────────────────────────────

  Future<void> _loadEntry() async {
    setState(() => _loading = true);
    try {
      final entry = await ref.read(apiClientProvider).getEntry(widget.editId!);
      _ctrl['name']!.text = entry.name;
      _ctrl['url']!.text = entry.url;
      setState(() => _type = entry.type);

      final authSvc = ref.read(authServiceProvider);
      if (authSvc.privX25519 != null && entry.entryKey != null) {
        final dataKey = await VaultCrypto.decryptDataKey(
          entry.entryKey!.encryptedKey,
          authSvc.privX25519!,
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
        // Custom fields
        final raw = data['custom_fields'];
        if (raw is List) {
          for (final cf in raw) {
            if (cf is Map<String, dynamic>) {
              _customFields.add((
                TextEditingController(text: cf['label'] as String? ?? ''),
                TextEditingController(text: cf['value'] as String? ?? ''),
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
    final Map<String, dynamic> data = {};
    for (final key in _allFields) {
      final val = _ctrl[key]!.text.trim();
      if (val.isNotEmpty) data[key] = val;
    }
    if (_type == 'bank-account' && _accountType.isNotEmpty) {
      data['account_type'] = _accountType;
    }
    if (_customFields.isNotEmpty) {
      data['custom_fields'] = [
        for (final (l, v) in _customFields)
          if (l.text.trim().isNotEmpty)
            {'label': l.text.trim(), 'value': v.text.trim()},
      ];
    }
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
      if (myPubX25519 == null) throw Exception('Public key not found');
      final myUserId = await authSvc.getUserId();
      final encKey = await VaultCrypto.encryptDataKey(dataKey, myPubX25519);

      if (isEdit) {
        await ref.read(apiClientProvider).updateEntry(
          widget.editId!,
          UpdateEntryRequest(
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
          ),
        );
      }
      if (mounted) context.go('/entries');
    } catch (e) {
      setState(() => _error = e.toString());
    } finally {
      if (mounted) setState(() => _loading = false);
    }
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

  List<Widget> _passwordFields() => [
    _field('username', 'Username', Icons.person_outline),
    _passwordField(),
    _field('totp_secret', 'TOTP Secret (optional, base32)', Icons.schedule),
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
        Row(children: [
          Expanded(
            child: PbTextField(
              label: 'Label',
              controller: _customFields[i].$1,
              prefixIcon: Icons.label_outline,
            ),
          ),
          const SizedBox(width: 8),
          Expanded(
            flex: 2,
            child: PbTextField(
              label: 'Value',
              controller: _customFields[i].$2,
              prefixIcon: Icons.edit_outlined,
            ),
          ),
          IconButton(
            icon: const Icon(Icons.remove_circle_outline,
                color: AppTheme.error, size: 20),
            onPressed: () {
              _customFields[i].$1.dispose();
              _customFields[i].$2.dispose();
              setState(() => _customFields.removeAt(i));
            },
          ),
        ]),
      ],
      const SizedBox(height: 8),
      TextButton.icon(
        icon: const Icon(Icons.add, size: 16),
        label: const Text('Add custom field'),
        style: TextButton.styleFrom(foregroundColor: AppTheme.green),
        onPressed: () => setState(() =>
            _customFields.add((TextEditingController(), TextEditingController()))),
      ),
    ],
  );

  // ── Build ─────────────────────────────────────────────────────────────────

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: Text(isEdit ? '> EDIT ENTRY' : '> NEW ENTRY')),
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
