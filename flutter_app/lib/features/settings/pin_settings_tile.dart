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
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/auth/auth_service.dart';
import '../../core/crypto/pin_crypto.dart';
import '../../core/theme/app_theme.dart';

/// Settings tile that manages PIN quick-unlock (enable / change / disable).
class PinSettingsTile extends ConsumerStatefulWidget {
  const PinSettingsTile({super.key});

  @override
  ConsumerState<PinSettingsTile> createState() => _PinSettingsTileState();
}

class _PinSettingsTileState extends ConsumerState<PinSettingsTile> {
  PinStatus? _status;

  @override
  void initState() {
    super.initState();
    _refresh();
  }

  Future<void> _refresh() async {
    final s = await ref.read(authStateProvider.notifier).pinStatus();
    if (mounted) setState(() => _status = s);
  }

  @override
  Widget build(BuildContext context) {
    final status = _status;
    final enabled = status?.enabled ?? false;
    final subtitle = status == null
        ? 'Checking…'
        : enabled
            ? 'On — master password required every ${status.intervalDays} days'
            : 'Off — unlock faster with a PIN';

    return ListTile(
      leading: const Icon(Icons.pin_outlined),
      title: const Text('PIN quick-unlock'),
      subtitle: Text(subtitle),
      trailing: Switch(
        value: enabled,
        activeThumbColor: AppTheme.green,
        onChanged: (v) => v ? _setup() : _confirmDisable(),
      ),
      onTap: enabled ? _showManage : _setup,
    );
  }

  Future<void> _showManage() async {
    final action = await showModalBottomSheet<String>(
      context: context,
      builder: (ctx) => SafeArea(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            ListTile(
              leading: const Icon(Icons.password),
              title: const Text('Change PIN'),
              onTap: () => Navigator.of(ctx).pop('change'),
            ),
            ListTile(
              leading: const Icon(Icons.no_encryption_gmailerrorred_outlined),
              title: const Text('Disable PIN'),
              onTap: () => Navigator.of(ctx).pop('disable'),
            ),
          ],
        ),
      ),
    );
    if (action == 'change') await _setup();
    if (action == 'disable') await _confirmDisable();
  }

  Future<void> _confirmDisable() async {
    await ref.read(authStateProvider.notifier).disablePin();
    await _refresh();
  }

  Future<void> _setup() async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (_) => const _PinSetupDialog(),
    );
    if (ok == true) await _refresh();
  }
}

/// Dialog to enable/change the PIN: master password + PIN + confirm + interval.
class _PinSetupDialog extends ConsumerStatefulWidget {
  const _PinSetupDialog();

  @override
  ConsumerState<_PinSetupDialog> createState() => _PinSetupDialogState();
}

class _PinSetupDialogState extends ConsumerState<_PinSetupDialog> {
  final _passCtrl = TextEditingController();
  final _pinCtrl = TextEditingController();
  final _pin2Ctrl = TextEditingController();
  int _intervalDays = PinCrypto.defaultIntervalDays;
  String? _error;
  bool _busy = false;

  @override
  void dispose() {
    _passCtrl.dispose();
    _pinCtrl.dispose();
    _pin2Ctrl.dispose();
    super.dispose();
  }

  Future<void> _save() async {
    setState(() => _error = null);
    if (_pinCtrl.text != _pin2Ctrl.text) {
      setState(() => _error = 'PINs do not match');
      return;
    }
    if (_pinCtrl.text.length < 4) {
      setState(() => _error = 'PIN must be at least 4 digits');
      return;
    }
    setState(() => _busy = true);
    try {
      final ok = await ref.read(authStateProvider.notifier).enablePin(
            _passCtrl.text,
            _pinCtrl.text,
            _intervalDays,
          );
      if (!mounted) return;
      if (ok) {
        Navigator.of(context).pop(true);
      } else {
        setState(() {
          _busy = false;
          _error = 'Wrong master password';
        });
      }
    } catch (e) {
      if (mounted) setState(() { _busy = false; _error = e.toString(); });
    }
  }

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      title: const Text('Set up PIN'),
      content: SingleChildScrollView(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            const Text(
              'A PIN unlocks faster but is weaker than your master password. '
              'It is stored on this device (in the OS keystore). Use a PIN you '
              "don't use elsewhere.",
              style: TextStyle(fontSize: 12, color: AppTheme.onBgDim),
            ),
            const SizedBox(height: 16),
            TextField(
              controller: _passCtrl,
              obscureText: true,
              decoration: const InputDecoration(labelText: 'Master password'),
            ),
            const SizedBox(height: 8),
            TextField(
              controller: _pinCtrl,
              obscureText: true,
              keyboardType: TextInputType.number,
              inputFormatters: [FilteringTextInputFormatter.digitsOnly],
              decoration: const InputDecoration(labelText: 'New PIN (digits)'),
            ),
            const SizedBox(height: 8),
            TextField(
              controller: _pin2Ctrl,
              obscureText: true,
              keyboardType: TextInputType.number,
              inputFormatters: [FilteringTextInputFormatter.digitsOnly],
              decoration: const InputDecoration(labelText: 'Confirm PIN'),
            ),
            const SizedBox(height: 16),
            Text('Require master password every $_intervalDays days',
                style: const TextStyle(fontSize: 12, color: AppTheme.onBgDim)),
            Slider(
              value: _intervalDays.toDouble(),
              min: PinCrypto.minIntervalDays.toDouble(),
              max: PinCrypto.maxIntervalDays.toDouble(),
              divisions: PinCrypto.maxIntervalDays - PinCrypto.minIntervalDays,
              label: '$_intervalDays days',
              activeColor: AppTheme.green,
              onChanged: (v) => setState(() => _intervalDays = v.round()),
            ),
            if (_error != null) ...[
              const SizedBox(height: 8),
              Text(_error!, style: const TextStyle(color: AppTheme.error)),
            ],
          ],
        ),
      ),
      actions: [
        TextButton(
          onPressed: _busy ? null : () => Navigator.of(context).pop(false),
          child: const Text('Cancel'),
        ),
        TextButton(
          onPressed: _busy ? null : _save,
          child: Text(_busy ? 'Saving…' : 'Save PIN'),
        ),
      ],
    );
  }
}
