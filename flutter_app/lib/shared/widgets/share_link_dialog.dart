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

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:qr_flutter/qr_flutter.dart';

import '../../core/theme/app_theme.dart';

/// Expiry choices for a share link. A null [validity] means "never expires".
class _ExpiryOption {
  final String label;
  final Duration? validity;
  const _ExpiryOption(this.label, this.validity);
}

const _expiryOptions = <_ExpiryOption>[
  _ExpiryOption('1 day', Duration(days: 1)),
  _ExpiryOption('7 days', Duration(days: 7)),
  _ExpiryOption('30 days', Duration(days: 30)),
  _ExpiryOption('90 days', Duration(days: 90)),
  _ExpiryOption('1 year', Duration(days: 365)),
  _ExpiryOption('Never', null),
];

/// Dialog that lets the user pick an expiry, creates the share link via
/// [onCreate], then shows the resulting URL in a terminal-styled field with a
/// copy button. [onCreate] receives the chosen validity (null = never) and
/// returns the shareable URL.
class ShareLinkDialog extends StatefulWidget {
  final String title;
  final Future<String> Function(Duration? validity) onCreate;

  const ShareLinkDialog({super.key, required this.title, required this.onCreate});

  @override
  State<ShareLinkDialog> createState() => _ShareLinkDialogState();
}

class _ShareLinkDialogState extends State<ShareLinkDialog> {
  int _expiryIndex = 1; // default: 7 days
  bool _busy = false;
  String? _url;
  String? _error;

  Future<void> _create() async {
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      final url = await widget.onCreate(_expiryOptions[_expiryIndex].validity);
      if (mounted) setState(() => _url = url);
    } catch (e) {
      if (mounted) setState(() => _error = '$e');
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  void _copy() {
    Clipboard.setData(ClipboardData(text: _url!));
    ScaffoldMessenger.of(context).showSnackBar(
      const SnackBar(content: Text('Link copied')),
    );
  }

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      title: Text(_url == null ? 'Share "${widget.title}"' : 'Share link'),
      content: _url == null
          ? _buildConfig()
          : SingleChildScrollView(child: _buildResult()),
      actions: _url == null
          ? [
              TextButton(
                onPressed: _busy ? null : () => Navigator.of(context).pop(),
                child: const Text('Cancel'),
              ),
              ElevatedButton(
                onPressed: _busy ? null : _create,
                child: _busy
                    ? const SizedBox(
                        width: 16, height: 16, child: CircularProgressIndicator(strokeWidth: 2))
                    : const Text('Create link'),
              ),
            ]
          : [
              TextButton(onPressed: _copy, child: const Text('Copy link')),
              TextButton(
                onPressed: () => Navigator.of(context).pop(),
                child: const Text('Done'),
              ),
            ],
    );
  }

  Widget _buildConfig() {
    return Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        const Text('How long should the link work?'),
        const SizedBox(height: 12),
        DropdownButtonFormField<int>(
          initialValue: _expiryIndex,
          isExpanded: true,
          decoration: const InputDecoration(
            labelText: 'Expires after',
            prefixIcon: Icon(Icons.schedule),
            border: OutlineInputBorder(),
          ),
          items: [
            for (var i = 0; i < _expiryOptions.length; i++)
              DropdownMenuItem(value: i, child: Text(_expiryOptions[i].label)),
          ],
          onChanged: _busy ? null : (v) => setState(() => _expiryIndex = v ?? 1),
        ),
        if (_error != null) ...[
          const SizedBox(height: 12),
          Text(_error!, style: const TextStyle(color: AppTheme.error, fontSize: 12)),
        ],
      ],
    );
  }

  Widget _buildResult() {
    return Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          _expiryOptions[_expiryIndex].validity == null
              ? 'This link never expires. Anyone with it can view the item.'
              : 'Valid for ${_expiryOptions[_expiryIndex].label}. The key is in the '
                  'link (after #) and never reaches the server.',
          style: const TextStyle(fontSize: 13),
        ),
        const SizedBox(height: 12),
        // Scannable QR of the full link (white quiet zone for reliable scans).
        Center(
          child: Container(
            padding: const EdgeInsets.all(12),
            decoration: BoxDecoration(
              color: Colors.white,
              borderRadius: BorderRadius.circular(8),
            ),
            // Fixed SizedBox so AlertDialog's IntrinsicWidth pass doesn't probe
            // QrImageView's internal LayoutBuilder (which can't be measured).
            child: SizedBox(
              width: 180,
              height: 180,
              child: QrImageView(
                data: _url!,
                version: QrVersions.auto,
                backgroundColor: Colors.white,
                // High error-correction so the QR still scans if slightly covered.
                errorCorrectionLevel: QrErrorCorrectLevel.M,
              ),
            ),
          ),
        ),
        const SizedBox(height: 12),
        // Terminal-styled link field: green on black, monospace, selectable.
        Container(
          width: double.infinity,
          padding: const EdgeInsets.all(10),
          decoration: BoxDecoration(
            color: Colors.black,
            border: Border.all(color: AppTheme.green),
            borderRadius: BorderRadius.circular(4),
          ),
          child: SelectableText(
            _url!,
            style: AppTheme.mono(
              color: AppTheme.green,
              fontSize: 12,
              height: 1.4,
            ),
          ),
        ),
      ],
    );
  }
}
