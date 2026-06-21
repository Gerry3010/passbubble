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
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';

import '../../../core/autofill/autofill_service.dart';
import '../../../core/theme/app_theme.dart';

/// Session-only dismissal of the vault banner. Reappears on next launch until
/// Passbubble is actually set as the autofill provider.
final autofillBannerDismissedProvider = StateProvider<bool>((_) => false);

/// Semi-permanent banner shown atop the vault while Passbubble is NOT the
/// active system autofill provider. Collapses to nothing when enabled,
/// unsupported, or dismissed for the session. The parent screen refreshes
/// [autofillEnabledProvider] on resume, so this reflects picker changes.
class AutofillBanner extends ConsumerWidget {
  const AutofillBanner({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final supported = ref.watch(autofillSupportedProvider).value ?? false;
    final enabled = ref.watch(autofillEnabledProvider).value ?? false;
    final dismissed = ref.watch(autofillBannerDismissedProvider);
    if (!supported || enabled || dismissed) return const SizedBox.shrink();

    return Container(
      margin: const EdgeInsets.fromLTRB(8, 8, 8, 0),
      padding: const EdgeInsets.fromLTRB(12, 10, 8, 10),
      decoration: BoxDecoration(
        color: AppTheme.surface,
        border: Border.all(color: AppTheme.amber),
        borderRadius: AppTheme.radiusMd,
      ),
      child: Row(
        children: [
          const Icon(Icons.warning_amber_rounded,
              color: AppTheme.amber, size: 18),
          const SizedBox(width: 10),
          const Expanded(
            child: Text(
              'Autofill is off — Passbubble can fill passwords in other apps.',
              style: TextStyle(fontSize: 12, color: AppTheme.onBg),
            ),
          ),
          TextButton(
            onPressed: () => ref.read(autofillBridgeProvider).requestEnable(),
            child: const Text('Enable'),
          ),
          IconButton(
            icon: const Icon(Icons.close, size: 16, color: AppTheme.onBgDim),
            visualDensity: VisualDensity.compact,
            onPressed: () =>
                ref.read(autofillBannerDismissedProvider.notifier).state = true,
          ),
        ],
      ),
    );
  }
}

/// Persists whether the one-time autofill intro has been shown.
class _AutofillIntroStore {
  static const _storage = FlutterSecureStorage(
    aOptions: AndroidOptions(encryptedSharedPreferences: true),
  );
  static const _key = 'autofill_intro_seen';

  static Future<bool> seen() async =>
      (await _storage.read(key: _key)) == '1';
  static Future<void> markSeen() async =>
      _storage.write(key: _key, value: '1');
}

/// Shows the one-time autofill intro sheet — once ever, after the first unlock,
/// and only when autofill is supported but not yet enabled. Safe to call from a
/// post-frame callback; no-ops otherwise.
Future<void> maybeShowAutofillIntro(
    BuildContext context, WidgetRef ref) async {
  final bridge = ref.read(autofillBridgeProvider);
  if (!await bridge.isSupported()) return;
  if (await bridge.isEnabled()) return;
  if (await _AutofillIntroStore.seen()) return;
  await _AutofillIntroStore.markSeen();
  if (!context.mounted) return;

  await showModalBottomSheet<void>(
    context: context,
    backgroundColor: AppTheme.surface,
    isScrollControlled: true,
    builder: (ctx) => Padding(
      padding: const EdgeInsets.fromLTRB(20, 20, 20, 28),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              const Icon(Icons.password_outlined,
                  color: AppTheme.green, size: 22),
              const SizedBox(width: 10),
              Text('# set up autofill',
                  style: AppTheme.mono(
                      color: AppTheme.green,
                      fontSize: 16,
                      fontWeight: FontWeight.w700)),
            ],
          ),
          const SizedBox(height: 14),
          const Text(
            'Let Passbubble fill your logins in other apps and the browser. '
            'Android needs you to pick Passbubble as the system autofill '
            'provider once — your vault must be unlocked for it to fill.',
            style: TextStyle(color: AppTheme.onBg, fontSize: 13, height: 1.4),
          ),
          const SizedBox(height: 20),
          Row(
            children: [
              Expanded(
                child: OutlinedButton(
                  onPressed: () => Navigator.of(ctx).pop(),
                  child: const Text('Later'),
                ),
              ),
              const SizedBox(width: 12),
              Expanded(
                child: ElevatedButton(
                  onPressed: () {
                    Navigator.of(ctx).pop();
                    ref.read(autofillBridgeProvider).requestEnable();
                  },
                  child: const Text('Enable now'),
                ),
              ),
            ],
          ),
        ],
      ),
    ),
  );
}
