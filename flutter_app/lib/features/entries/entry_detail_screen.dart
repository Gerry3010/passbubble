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

import 'dart:async';
import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:share_plus/share_plus.dart';

import '../../core/api/api_client.dart';
import '../../core/api/models.dart';
import '../../core/auth/auth_service.dart';
import '../../core/crypto/vault_crypto.dart';
import '../../core/theme/app_theme.dart';
import '../../core/util/entry_tags.dart';
import '../../core/util/entry_totp.dart';
import '../../shared/widgets/prompt_title.dart';
import '../../shared/widgets/share_link_dialog.dart';
import '../manage/shares_tab.dart' show sharesProvider;
import 'entries_list_screen.dart' show entriesProvider;
import 'widgets/history_sheet.dart';

final _entryDetailProvider =
    FutureProvider.autoDispose.family<EntryResponse, String>((ref, id) async {
  return ref.watch(apiClientProvider).getEntry(id);
});

class EntryDetailScreen extends ConsumerStatefulWidget {
  final String id;
  const EntryDetailScreen({super.key, required this.id});

  @override
  ConsumerState<EntryDetailScreen> createState() => _EntryDetailScreenState();
}

class _EntryDetailScreenState extends ConsumerState<EntryDetailScreen> {
  Map<String, dynamic>? _decrypted;
  bool _decrypting = false;
  // id@updatedAt of the entry version auto-decrypt last ran for, so it runs
  // once per loaded version (and again after an edit or restore).
  String? _autoDecryptedFor;
  // Secret rows the user has revealed, by row id — each eye toggles only its
  // own row.
  final Set<String> _revealed = {};
  String? _totpCode;
  int _totpRemaining = 30;
  Timer? _totpTimer;

  @override
  void dispose() {
    _totpTimer?.cancel();
    super.dispose();
  }

  Future<void> _decrypt(EntryResponse entry) async {
    final authSvc = ref.read(authServiceProvider);
    if (authSvc.privX25519 == null) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('Vault is locked — unlock first')),
      );
      return;
    }
    setState(() => _decrypting = true);
    try {
      final encKey = entry.entryKey;
      if (encKey == null) throw Exception('No entry key');
      final dataKey = await VaultCrypto.decryptDataKey(
        encKey.encryptedKey,
        authSvc.privX25519!,
        authSvc.privMLKEM!,
      );
      final data = await VaultCrypto.decryptEntryData(
        entry.encryptedData,
        Uint8List.fromList(dataKey),
      );
      setState(() => _decrypted = data);
      final totpSecret = totpSecretOf(data);
      if (totpSecret != null) {
        _startTOTP(totpSecret, TotpOptions.stored(data));
      } else {
        _stopTOTP(); // a restored version may have none any more
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text('Decrypt failed: $e')),
        );
      }
    } finally {
      if (mounted) setState(() => _decrypting = false);
    }
  }

  /// Decrypts [entry] right after it loads, without a tap on DECRYPT, as long
  /// as the vault is unlocked. DECRYPT stays as the fallback for a locked vault
  /// or a failed attempt (which is not retried automatically).
  void _maybeAutoDecrypt(EntryResponse entry) {
    final version = '${entry.id}@${entry.updatedAt}';
    if (_autoDecryptedFor == version || _decrypting) return;
    if (ref.read(authServiceProvider).privX25519 == null) return;
    _autoDecryptedFor = version;
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) _decrypt(entry);
    });
  }

  void _startTOTP(String secret, TotpOptions options) {
    _totpTimer?.cancel();
    _totpTimer = null;
    // A secret that yields no code gets no widget and no timer.
    if (!_updateTOTP(secret, options)) return;
    _totpTimer = Timer.periodic(const Duration(seconds: 1), (_) {
      if (mounted) _updateTOTP(secret, options);
    });
  }

  void _stopTOTP() {
    _totpTimer?.cancel();
    _totpTimer = null;
    setState(() => _totpCode = null);
  }

  /// Refreshes the code; returns whether [secret] produced one.
  bool _updateTOTP(String secret, TotpOptions options) {
    final totp = totpCodeAt(secret, options, DateTime.now().millisecondsSinceEpoch);
    setState(() {
      _totpCode = totp?.code;
      if (totp != null) _totpRemaining = totp.remaining;
    });
    return totp != null;
  }

  void _copy(String value, String label) {
    Clipboard.setData(ClipboardData(text: value));
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(content: Text('$label copied')),
    );
  }

  /// Creates a zero-knowledge share link for the (already decrypted) entry. The
  /// entry payload is encrypted with a fresh random key that is placed only in
  /// the URL fragment (after '#'), so the server never sees it.
  Future<void> _createShareLink(EntryResponse entry) async {
    // Decrypt on demand so the share action works straight from the list/detail
    // without the user having to tap DECRYPT first.
    if (_decrypted == null) {
      await _decrypt(entry);
      if (!mounted) return;
    }
    final d = _decrypted;
    if (d == null) return; // vault locked / decrypt failed
    final priv = ref.read(authServiceProvider).privX25519;
    if (priv == null) return;

    await showDialog<void>(
      context: context,
      builder: (_) => ShareLinkDialog(
        title: entry.name,
        onCreate: (validity) => _buildEntryShareLink(entry, d, priv, validity),
      ),
    );
  }

  /// Creates a new, independent share link for the entry and returns the
  /// shareable URL. A null [validity] means the link never expires (a far-future
  /// date). Each call mints a fresh link, so several can coexist for one entry.
  Future<String> _buildEntryShareLink(
    EntryResponse entry,
    Map<String, dynamic> data,
    Uint8List priv,
    Duration? validity,
  ) async {
    final payload = {
      'name': entry.name,
      'type': entry.type,
      'url': entry.url,
      'data': data,
    };
    // Fresh random link key per link → each shared URL is independent.
    final linkKey = VaultCrypto.randomKey();
    final encryptedPayload =
        await VaultCrypto.encryptShareLinkPayload(linkKey, payload);
    final exp = validity == null
        ? DateTime.utc(2125)
        : DateTime.now().toUtc().add(validity);
    final expStr = '${exp.toIso8601String().split('.').first}Z';

    final api = ref.read(apiClientProvider);
    final link = await api.createEntryShareLink(
      entry.id,
      CreateShareLinkRequest(
        encryptedPayload: encryptedPayload,
        payloadNonce: base64.encode(Uint8List(12)), // placeholder; nonce is in the ciphertext
        expiresAt: expStr,
      ),
    );
    ref.invalidate(sharesProvider);

    final secret = base64Url.encode(linkKey);
    return '${api.publicBaseUrl}/web/#/share/${link.token}?k=${Uri.encodeQueryComponent(secret)}';
  }

  Future<void> _toggleFavorite(EntryResponse entry) async {
    try {
      await ref.read(apiClientProvider).setFavorite(entry.id, !entry.favorite);
      ref.invalidate(_entryDetailProvider(widget.id));
      ref.invalidate(entriesProvider);
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context)
            .showSnackBar(SnackBar(content: Text('Failed: $e')));
      }
    }
  }

  Future<void> _delete(String id) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('Move to trash?'),
        content: const Text(
            'The entry moves to the trash and can be restored for 30 days '
            '(Settings → Trash).'),
        actions: [
          TextButton(
              onPressed: () => ctx.pop(false), child: const Text('Cancel')),
          TextButton(
            onPressed: () => ctx.pop(true),
            child: const Text('Delete', style: TextStyle(color: AppTheme.error)),
          ),
        ],
      ),
    );
    if (ok != true) return;
    await ref.read(apiClientProvider).deleteEntry(id);
    ref.invalidate(entriesProvider);
    ref.read(authServiceProvider).refreshAutofill().ignore();
    if (mounted) context.go('/entries');
  }

  @override
  Widget build(BuildContext context) {
    final async = ref.watch(_entryDetailProvider(widget.id));
    // Rebuild on unlock, so a detail page opened while locked decrypts then.
    ref.watch(vaultSessionProvider);
    return Scaffold(
      appBar: AppBar(
        title: async.when(
          data: (e) => PromptTitle(e.name.toLowerCase()),
          loading: () => const PromptTitle('loading…'),
          error: (_, _) => const PromptTitle('error'),
        ),
        actions: [
          if (async.hasValue) ...[
            IconButton(
              icon: Icon(
                async.value!.favorite ? Icons.star : Icons.star_border,
                color: async.value!.favorite ? Colors.amber : null,
              ),
              tooltip: async.value!.favorite
                  ? 'Remove from favorites'
                  : 'Add to favorites',
              onPressed: () => _toggleFavorite(async.value!),
            ),
            IconButton(
              icon: const Icon(Icons.history),
              tooltip: 'Version history',
              onPressed: () => showHistorySheet(
                context,
                ref,
                async.value!,
                onRestored: () {
                  ref.invalidate(_entryDetailProvider(widget.id));
                  ref.invalidate(entriesProvider);
                  setState(() => _decrypted = null);
                },
              ),
            ),
            IconButton(
              icon: const Icon(Icons.ios_share),
              tooltip: 'Create share link',
              onPressed: () => _createShareLink(async.value!),
            ),
            IconButton(
              icon: const Icon(Icons.edit_outlined),
              onPressed: () => context.go('/entries/${widget.id}/edit'),
            ),
            IconButton(
              icon: const Icon(Icons.delete_outline, color: AppTheme.error),
              onPressed: () => _delete(widget.id),
            ),
          ],
        ],
      ),
      body: async.when(
        loading: () => const Center(child: CircularProgressIndicator()),
        error: (e, _) => Center(
            child: Text(e.toString(),
                style: const TextStyle(color: AppTheme.error))),
        data: (entry) => _buildDetail(entry),
      ),
    );
  }

  Widget _buildDetail(EntryResponse entry) {
    _maybeAutoDecrypt(entry);
    final d = _decrypted;
    // Tags live in the encrypted data, so they show once decrypted.
    final tags = d == null ? const <String>[] : tagsOf(d);
    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        // Type chip
        Wrap(
          spacing: 8,
          children: [
            Chip(label: Text(entry.type.toUpperCase())),
            if (entry.permission != 'owner')
              Chip(
                label: Text(entry.permission.toUpperCase()),
                side: const BorderSide(color: AppTheme.green),
              ),
          ],
        ),
        if (tags.isNotEmpty) ...[
          const SizedBox(height: 8),
          _tagChips(tags),
        ],
        const SizedBox(height: 16),
        if (entry.url.isNotEmpty) _infoRow('URL', entry.url),
        const Divider(),

        // TOTP display (shown whenever an entry has a totp_secret, not just type=='totp')
        if (_totpCode != null) ...[
          _totpWidget(),
          const Divider(),
        ],

        // Decrypted fields
        if (d == null) ...[
          const SizedBox(height: 24),
          Center(
            child: _decrypting
                ? const CircularProgressIndicator()
                : ElevatedButton.icon(
                    onPressed: () => _decrypt(entry),
                    icon: const Icon(Icons.lock_open_outlined),
                    label: const Text('DECRYPT'),
                  ),
          ),
        ] else
          ..._buildDecryptedFields(entry.type, d),
        const SizedBox(height: 80),
      ],
    );
  }

  /// The entry's tags; tapping one lists every entry carrying it.
  Widget _tagChips(List<String> tags) => Wrap(
        spacing: 8,
        runSpacing: 4,
        children: [
          for (final tag in tags)
            ActionChip(
              avatar: const Icon(Icons.tag, size: 14, color: AppTheme.green),
              label: Text(tag),
              tooltip: 'Show entries tagged $tag',
              onPressed: () => context.go(
                  Uri(path: '/entries', queryParameters: {'tag': tag}).toString()),
            ),
        ],
      );

  // ── Type-specific field renderers ─────────────────────────────────────────

  static String _ssoLabel(String provider) => switch (provider) {
        'google' => 'Google',
        'apple' => 'Apple',
        'microsoft' => 'Microsoft',
        'github' => 'GitHub',
        'facebook' => 'Facebook',
        _ => provider,
      };

  List<Widget> _buildDecryptedFields(String type, Map<String, dynamic> d) {
    String? str(String key) {
      final v = d[key];
      if (v is String && v.isNotEmpty) return v;
      return null;
    }

    final totpSecret = totpSecretOf(d);
    final customFieldWidgets = <Widget>[];
    final rawCf = d['custom_fields'];
    if (rawCf is List && rawCf.isNotEmpty) {
      customFieldWidgets.add(const Padding(
        padding: EdgeInsets.only(top: 16, bottom: 4),
        child: Text('CUSTOM FIELDS',
            style: TextStyle(color: AppTheme.green, fontSize: 11, letterSpacing: 2)),
      ));
      for (final (i, cf) in rawCf.indexed) {
        if (cf is Map<String, dynamic>) {
          final label = cf['label'] as String? ?? '';
          final value = cf['value'] as String? ?? '';
          final cfType = cf['type'] as String? ?? 'text';
          if (label.isEmpty) continue;
          switch (cfType) {
            case 'password':
            case 'ssh':
              customFieldWidgets.add(_secretRow(label, value, id: 'cf:$i'));
            case 'totp':
              if (value.isNotEmpty && _totpCode == null) {
                _startTOTP(value, TotpOptions.stored(d));
              }
              customFieldWidgets.add(_secretRow(label, value, id: 'cf:$i'));
            case 'file':
              final filename = cf['filename'] as String? ?? label;
              final mime = cf['mime_type'] as String? ?? 'application/octet-stream';
              customFieldWidgets.add(_fileRow(label, value, filename, mime));
            default:
              customFieldWidgets.add(_copyRow(label, value));
          }
        }
      }
    }

    final common = <Widget>[
      if (str('notes') != null) _infoRow('Notes', str('notes')!),
      ...customFieldWidgets,
    ];

    return switch (type) {
      'password' || 'api-key' => [
          if (str('username') != null) _copyRow('Username', str('username')!),
          if (str('password') != null) _secretRow('Password', str('password')!, id: 'password'),
          if (totpSecret != null) _secretRow('TOTP Secret', totpSecret, id: 'totp_secret'),
          if (str('sign_in_with') != null)
            _infoRow('Sign in with', _ssoLabel(str('sign_in_with')!)),
          ...common,
        ],
      'totp' => [
          if (str('username') != null) _copyRow('Account', str('username')!),
          if (str('password') != null) _secretRow('Password', str('password')!, id: 'password'),
          if (totpSecret != null) _secretRow('TOTP Secret', totpSecret, id: 'totp_secret'),
          ...common,
        ],
      'note' => [
          if (str('notes') != null) _infoRow('Content', str('notes')!),
          ...customFieldWidgets,
        ],
      'ssh-key' => [
          if (str('username') != null) _copyRow('User', str('username')!),
          if (str('password') != null) _secretRow('Private Key', str('password')!, id: 'password'),
          ...common,
        ],
      'credit-card' => [
          if (str('card_number') != null)
            _maskedCardRow('Card Number', str('card_number')!, id: 'card_number'),
          if (str('holder_name') != null) _infoRow('Cardholder', str('holder_name')!),
          if (str('expiry_month') != null || str('expiry_year') != null)
            _infoRow('Expires',
                '${str('expiry_month') ?? '??'}/${str('expiry_year') ?? '????'}'),
          if (str('cvv') != null) _secretRow('CVV', str('cvv')!, id: 'cvv'),
          ...common,
        ],
      'bank-account' => [
          if (str('bank_name') != null) _infoRow('Bank', str('bank_name')!),
          if (str('iban') != null) _copyRow('IBAN', str('iban')!),
          if (str('bic') != null) _copyRow('BIC / SWIFT', str('bic')!),
          if (str('account_number') != null)
            _copyRow('Account Number', str('account_number')!),
          if (str('account_type') != null) _infoRow('Type', str('account_type')!),
          ...common,
        ],
      'identity' => [
          if (str('title') != null || str('first_name') != null || str('last_name') != null)
            _infoRow('Name',
                [str('title'), str('first_name'), str('last_name')]
                    .where((v) => v != null)
                    .join(' ')),
          if (str('company') != null) _infoRow('Company', str('company')!),
          if (str('email') != null) _copyRow('Email', str('email')!),
          if (str('phone') != null) _copyRow('Phone', str('phone')!),
          if ([str('street'), str('city'), str('state'), str('postal_code'), str('country')]
              .any((v) => v != null))
            _infoRow('Address', _formatAddress(d)),
          ...common,
        ],
      'license' => [
          if (str('product_name') != null) _infoRow('Product', str('product_name')!),
          if (str('license_key') != null) _secretRow('License Key', str('license_key')!, id: 'license_key'),
          if (str('purchase_email') != null)
            _copyRow('Purchase Email', str('purchase_email')!),
          if (str('purchase_date') != null) _infoRow('Purchased', str('purchase_date')!),
          if (str('expires_at') != null) _infoRow('Expires', str('expires_at')!),
          ...common,
        ],
      _ => [
          if (str('username') != null) _copyRow('Username', str('username')!),
          if (str('password') != null) _secretRow('Password', str('password')!, id: 'password'),
          ...common,
        ],
    };
  }

  String _formatAddress(Map<String, dynamic> d) {
    final parts = <String>[];
    for (final key in ['street', 'city', 'state', 'postal_code', 'country']) {
      final v = d[key];
      if (v is String && v.isNotEmpty) parts.add(v);
    }
    return parts.join(', ');
  }

  Widget _maskedCardRow(String label, String number, {required String id}) {
    final masked = number.length >= 4
        ? '•••• •••• •••• ${number.substring(number.length - 4)}'
        : '••••••••';
    return ListTile(
      contentPadding: EdgeInsets.zero,
      title: Text(label, style: const TextStyle(color: AppTheme.onBgDim, fontSize: 12)),
      subtitle: _secretText(number, masked: masked, revealed: _revealed.contains(id)),
      trailing: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          IconButton(
            icon: Icon(
              _revealed.contains(id) ? Icons.visibility_off : Icons.visibility,
              size: 18,
              color: AppTheme.onBgDim,
            ),
            onPressed: () => _toggleReveal(id),
          ),
          IconButton(
            icon: const Icon(Icons.copy, size: 18, color: AppTheme.onBgDim),
            onPressed: () => _copy(number, label),
          ),
        ],
      ),
    );
  }

  Widget _totpWidget() {
    return Container(
      padding: const EdgeInsets.all(16),
      margin: const EdgeInsets.symmetric(vertical: 8),
      decoration: BoxDecoration(
        border: Border.all(color: AppTheme.green),
        color: AppTheme.greenFaint,
      ),
      child: Row(
        children: [
          Expanded(
            child: Text(
              _formatTOTP(_totpCode ?? '------'),
              style: const TextStyle(
                fontSize: 32,
                fontWeight: FontWeight.w700,
                color: AppTheme.green,
                letterSpacing: 8,
              ),
            ),
          ),
          Column(
            children: [
              Text(
                '$_totpRemaining',
                style: const TextStyle(
                  color: AppTheme.green,
                  fontSize: 20,
                  fontWeight: FontWeight.w600,
                ),
              ),
              const Text('sec', style: TextStyle(color: AppTheme.onBgDim, fontSize: 11)),
            ],
          ),
          const SizedBox(width: 8),
          IconButton(
            icon: const Icon(Icons.copy, color: AppTheme.green),
            onPressed: () => _copy(_totpCode ?? '', 'TOTP code'),
          ),
        ],
      ),
    );
  }

  String _formatTOTP(String code) {
    if (code.length == 6) return '${code.substring(0, 3)} ${code.substring(3)}';
    if (code.length == 8) return '${code.substring(0, 4)} ${code.substring(4)}';
    return code;
  }

  Widget _fileRow(String label, String base64Value, String filename, String mime) {
    return ListTile(
      contentPadding: EdgeInsets.zero,
      leading: const Icon(Icons.insert_drive_file_outlined, color: AppTheme.onBgDim),
      title: Text(label, style: const TextStyle(color: AppTheme.onBgDim, fontSize: 12)),
      subtitle: _selectableValue(filename),
      trailing: IconButton(
        icon: const Icon(Icons.download_outlined, color: AppTheme.onBgDim),
        tooltip: 'Save file',
        onPressed: () async {
          try {
            final bytes = base64Decode(base64Value);
            // iOS requires a non-zero source rect for the share sheet/popover.
            final box = context.findRenderObject() as RenderBox?;
            final origin = box != null && box.hasSize
                ? box.localToGlobal(Offset.zero) & box.size
                : const Rect.fromLTWH(0, 0, 1, 1);
            await Share.shareXFiles(
              [XFile.fromData(Uint8List.fromList(bytes), name: filename, mimeType: mime)],
              subject: filename,
              sharePositionOrigin: origin,
            );
          } catch (e) {
            if (mounted) {
              ScaffoldMessenger.of(context)
                  .showSnackBar(SnackBar(content: Text('Cannot save file: $e')));
            }
          }
        },
      ),
    );
  }

  void _toggleReveal(String id) =>
      setState(() => _revealed.contains(id) ? _revealed.remove(id) : _revealed.add(id));

  /// A plaintext value the user can select (mouse drag / long-press) and copy.
  /// SelectableText is backed by a real text-editing surface, so selection and
  /// Ctrl/Cmd+C also work on web, where plain canvas text is not selectable.
  Widget _selectableValue(String value) =>
      SelectableText(value, style: const TextStyle(fontSize: 14));

  /// A secret value: plain (non-selectable) bullets while masked, so only the
  /// mask could ever be picked up; selectable like any other value once the
  /// user has revealed it.
  Widget _secretText(String value, {required String masked, required bool revealed}) {
    const style = TextStyle(fontSize: 14, letterSpacing: 2);
    return revealed
        ? SelectableText(value, style: style)
        : Text(masked, style: style);
  }

  Widget _infoRow(String label, String value) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 8),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(label, style: const TextStyle(color: AppTheme.onBgDim, fontSize: 11)),
          const SizedBox(height: 4),
          _selectableValue(value),
        ],
      ),
    );
  }

  Widget _copyRow(String label, String value) {
    return ListTile(
      contentPadding: EdgeInsets.zero,
      title: Text(label, style: const TextStyle(color: AppTheme.onBgDim, fontSize: 12)),
      subtitle: _selectableValue(value),
      trailing: IconButton(
        icon: const Icon(Icons.copy, size: 18, color: AppTheme.onBgDim),
        onPressed: () => _copy(value, label),
      ),
    );
  }

  Widget _secretRow(String label, String value, {required String id}) {
    return ListTile(
      contentPadding: EdgeInsets.zero,
      title: Text(label, style: const TextStyle(color: AppTheme.onBgDim, fontSize: 12)),
      subtitle: _secretText(value,
          masked: '•' * value.length.clamp(0, 20), revealed: _revealed.contains(id)),
      trailing: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          IconButton(
            icon: Icon(
              _revealed.contains(id) ? Icons.visibility_off : Icons.visibility,
              size: 18,
              color: AppTheme.onBgDim,
            ),
            onPressed: () => _toggleReveal(id),
          ),
          IconButton(
            icon: const Icon(Icons.copy, size: 18, color: AppTheme.onBgDim),
            onPressed: () => _copy(value, label),
          ),
        ],
      ),
    );
  }
}
