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

import 'package:cryptography/cryptography.dart';
import 'package:flutter/foundation.dart' show kIsWeb;
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/api/api_client.dart';
import '../../core/crypto/vault_crypto.dart';
import '../../core/theme/app_theme.dart';

/// Public, unauthenticated viewer for a share link. The decryption key [k] comes
/// from the URL fragment and never reaches the server; only [token] is sent to
/// the API. Optionally the link is protected by a password.
class ShareViewerScreen extends ConsumerStatefulWidget {
  final String token;
  final String k;

  const ShareViewerScreen({super.key, required this.token, required this.k});

  @override
  ConsumerState<ShareViewerScreen> createState() => _ShareViewerScreenState();
}

class _ShareViewerScreenState extends ConsumerState<ShareViewerScreen> {
  bool _loading = true;
  bool _needsPassword = false;
  String? _error;
  Map<String, dynamic>? _entry;
  final _passwordCtrl = TextEditingController();

  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  void dispose() {
    _passwordCtrl.dispose();
    super.dispose();
  }

  ApiClient get _api {
    final api = ref.read(apiClientProvider);
    if (!api.isConfigured && kIsWeb) {
      api.setBaseUrlEphemeral(Uri.base.origin);
    }
    return api;
  }

  Future<void> _load({String? password}) async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final resp = await _api.getPublicShareLink(widget.token, password: password);
      if (resp.requiresPassword && resp.encryptedPayload.isEmpty) {
        setState(() {
          _needsPassword = true;
          _loading = false;
        });
        return;
      }

      final key = SecretKey(base64Url.decode(widget.k));
      final plain = await VaultCrypto.decrypt(key, base64.decode(resp.encryptedPayload));
      final map = jsonDecode(utf8.decode(plain)) as Map<String, dynamic>;
      setState(() {
        _entry = map;
        _needsPassword = false;
        _loading = false;
      });
    } catch (e) {
      setState(() {
        _error = 'This link is invalid, expired, or the key is wrong.';
        _loading = false;
      });
    }
  }

  void _copy(String value, String label) {
    Clipboard.setData(ClipboardData(text: value));
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('$label copied')));
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('> SHARED ENTRY')),
      body: Center(
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 480),
          child: Padding(
            padding: const EdgeInsets.all(24),
            child: _buildBody(),
          ),
        ),
      ),
    );
  }

  Widget _buildBody() {
    if (_loading) return const Center(child: CircularProgressIndicator());
    if (_error != null) {
      return Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          const Icon(Icons.link_off, color: AppTheme.error, size: 48),
          const SizedBox(height: 12),
          Text(_error!, textAlign: TextAlign.center),
        ],
      );
    }
    if (_needsPassword) return _buildPasswordPrompt();
    if (_entry != null) {
      // A folder share link carries {folder, entries:[...]}; a single entry
      // carries {name, type, url, data}.
      final entries = _entry!['entries'];
      if (entries is List) return _buildFolder(_entry!, entries);
      return _buildEntry(_entry!);
    }
    return const SizedBox.shrink();
  }

  Widget _buildFolder(Map<String, dynamic> f, List<dynamic> entries) {
    final folderName = f['folder'] as String? ?? 'Shared folder';
    return ListView(
      shrinkWrap: true,
      children: [
        Text(folderName, style: Theme.of(context).textTheme.headlineSmall),
        Text('${entries.length} entries', style: const TextStyle(color: AppTheme.onBgDim)),
        const SizedBox(height: 12),
        for (final e in entries.whereType<Map<String, dynamic>>())
          Card(
            child: ExpansionTile(
              title: Text(e['name'] as String? ?? 'Entry'),
              childrenPadding: const EdgeInsets.symmetric(horizontal: 16),
              children: _entryFieldTiles(e),
            ),
          ),
        const SizedBox(height: 16),
        const Text(
          'Decrypted locally — the server never received the decryption key.',
          style: TextStyle(fontSize: 11, color: AppTheme.onBgDim),
        ),
      ],
    );
  }

  Widget _buildPasswordPrompt() {
    return Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        const Text('This link is password-protected.'),
        const SizedBox(height: 12),
        TextField(
          controller: _passwordCtrl,
          obscureText: true,
          decoration: const InputDecoration(labelText: 'Link password'),
          onSubmitted: (_) => _load(password: _passwordCtrl.text),
        ),
        const SizedBox(height: 16),
        ElevatedButton(
          onPressed: () => _load(password: _passwordCtrl.text),
          child: const Text('Unlock'),
        ),
      ],
    );
  }

  /// Builds copyable field tiles for one entry map ({name,type,url,data}).
  List<Widget> _entryFieldTiles(Map<String, dynamic> e) {
    final url = e['url'] as String? ?? '';
    final data = (e['data'] as Map<String, dynamic>?) ?? const {};
    final fields = <({String label, String value, bool secret})>[
      if (url.isNotEmpty) (label: 'URL', value: url, secret: false),
      if ((data['username'] as String?)?.isNotEmpty ?? false)
        (label: 'Username', value: data['username'] as String, secret: false),
      if ((data['password'] as String?)?.isNotEmpty ?? false)
        (label: 'Password', value: data['password'] as String, secret: true),
      if ((data['totp_secret'] as String?)?.isNotEmpty ?? false)
        (label: 'TOTP secret', value: data['totp_secret'] as String, secret: true),
      if ((data['notes'] as String?)?.isNotEmpty ?? false)
        (label: 'Notes', value: data['notes'] as String, secret: false),
    ];
    return [
      for (final f in fields)
        _FieldTile(
          label: f.label,
          value: f.value,
          secret: f.secret,
          onCopy: () => _copy(f.value, f.label),
        ),
    ];
  }

  Widget _buildEntry(Map<String, dynamic> e) {
    final name = e['name'] as String? ?? 'Shared entry';
    return ListView(
      shrinkWrap: true,
      children: [
        Text(name, style: Theme.of(context).textTheme.headlineSmall),
        const SizedBox(height: 16),
        ..._entryFieldTiles(e),
        const SizedBox(height: 16),
        const Text(
          'Decrypted locally — the server never received the decryption key.',
          style: TextStyle(fontSize: 11, color: AppTheme.onBgDim),
        ),
      ],
    );
  }
}

/// One field row in the public share viewer. Secret fields (password / TOTP
/// secret) are masked with a reveal toggle; every field can be copied.
class _FieldTile extends StatefulWidget {
  final String label;
  final String value;
  final bool secret;
  final VoidCallback onCopy;
  const _FieldTile({
    required this.label,
    required this.value,
    required this.secret,
    required this.onCopy,
  });

  @override
  State<_FieldTile> createState() => _FieldTileState();
}

class _FieldTileState extends State<_FieldTile> {
  bool _revealed = false;

  @override
  Widget build(BuildContext context) {
    final masked = !widget.secret || _revealed
        ? widget.value
        : '•' * widget.value.length.clamp(1, 20);
    return ListTile(
      contentPadding: EdgeInsets.zero,
      title: Text(widget.label,
          style: const TextStyle(fontSize: 12, color: AppTheme.onBgDim)),
      subtitle: Text(masked),
      trailing: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          if (widget.secret)
            IconButton(
              icon: Icon(
                _revealed ? Icons.visibility_off : Icons.visibility,
                size: 18,
                color: AppTheme.onBgDim,
              ),
              tooltip: _revealed ? 'Hide' : 'Show',
              onPressed: () => setState(() => _revealed = !_revealed),
            ),
          IconButton(
            icon: const Icon(Icons.copy, size: 18),
            onPressed: widget.onCopy,
          ),
        ],
      ),
    );
  }
}
