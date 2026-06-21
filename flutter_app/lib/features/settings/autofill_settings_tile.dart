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

import '../../core/autofill/autofill_service.dart';
import '../../core/theme/app_theme.dart';

/// Settings tile (under `# security`) showing whether Passbubble is the active
/// system autofill provider, with a tap-to-enable shortcut to the OS picker.
/// Renders nothing on platforms without autofill support.
class AutofillSettingsTile extends ConsumerStatefulWidget {
  const AutofillSettingsTile({super.key});

  @override
  ConsumerState<AutofillSettingsTile> createState() =>
      _AutofillSettingsTileState();
}

class _AutofillSettingsTileState extends ConsumerState<AutofillSettingsTile>
    with WidgetsBindingObserver {
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    // Re-check on open in case it was toggled outside the app.
    ref.invalidateLater(autofillEnabledProvider);
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    super.dispose();
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    // The user returns here after the system autofill picker — refresh status.
    if (state == AppLifecycleState.resumed) {
      ref.invalidate(autofillEnabledProvider);
    }
  }

  Future<void> _enable() async {
    await ref.read(autofillBridgeProvider).requestEnable();
    // Status refreshes via the resume lifecycle callback.
  }

  @override
  Widget build(BuildContext context) {
    final supported = ref.watch(autofillSupportedProvider).value ?? false;
    if (!supported) return const SizedBox.shrink();

    final enabled = ref.watch(autofillEnabledProvider).value ?? false;
    return ListTile(
      leading: Icon(
        Icons.password_outlined,
        color: enabled ? AppTheme.green : AppTheme.onBgDim,
      ),
      title: const Text('Autofill service'),
      subtitle: Text(
        enabled
            ? 'Passbubble fills passwords in other apps'
            : 'Set Passbubble as your system autofill provider',
        style: const TextStyle(color: AppTheme.onBgDim, fontSize: 12),
      ),
      trailing: enabled
          ? const Text('✓ active',
              style: TextStyle(color: AppTheme.green, fontSize: 12))
          : const Text('inactive ›',
              style: TextStyle(color: AppTheme.onBgDim, fontSize: 12)),
      onTap: enabled ? null : _enable,
    );
  }
}

/// Riverpod's `invalidate` cannot run during `initState`/build; defer it.
extension on WidgetRef {
  void invalidateLater(ProviderOrFamily provider) {
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (context.mounted) invalidate(provider);
    });
  }
}
