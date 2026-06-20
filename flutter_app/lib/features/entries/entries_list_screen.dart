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

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../core/api/api_client.dart';
import '../../core/api/models.dart';
import '../../core/auth/auth_service.dart';
import '../../core/crypto/vault_crypto.dart';
import '../../core/theme/app_theme.dart';
import '../../shared/widgets/share_link_dialog.dart';
import '../manage/shares_tab.dart' show sharesProvider;
import '../../shared/widgets/bottom_nav.dart';
import '../../shared/widgets/pb_button.dart';

final entriesProvider = FutureProvider<List<EntryResponse>>((ref) async {
  return ref.watch(apiClientProvider).listEntries();
});

final foldersProvider = FutureProvider<List<FolderResponse>>((ref) {
  return ref.watch(apiClientProvider).listFolders();
});

class EntriesListScreen extends ConsumerStatefulWidget {
  const EntriesListScreen({super.key});

  @override
  ConsumerState<EntriesListScreen> createState() => _EntriesListScreenState();
}

class _EntriesListScreenState extends ConsumerState<EntriesListScreen>
    with WidgetsBindingObserver {
  final _searchCtrl = TextEditingController();
  String _searchQuery = '';

  // Folder navigation stack — null means root level.
  final List<FolderResponse?> _stack = [null];

  // Multi-select edit mode (Apple-Mail style). When active, tapping a tile
  // toggles its selection and a bottom action toolbar operates on the set.
  bool _editMode = false;
  final Set<String> _selected = {};

  FolderResponse? get _currentFolder => _stack.last;
  bool get _atRoot => _stack.length == 1;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    _searchCtrl.dispose();
    super.dispose();
  }

  // Sync when returning to the app: entries saved elsewhere (the browser
  // extension or another device) show up without a manual pull-to-refresh.
  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state == AppLifecycleState.resumed) {
      ref.invalidate(entriesProvider);
      ref.invalidate(foldersProvider);
    }
  }

  void _exitEditMode() {
    if (!mounted) return;
    setState(() {
      _editMode = false;
      _selected.clear();
    });
  }

  void _toggleSelected(String id) {
    setState(() {
      if (!_selected.remove(id)) _selected.add(id);
    });
  }

  void _snack(String message) {
    if (!mounted) return;
    ScaffoldMessenger.of(context)
        .showSnackBar(SnackBar(content: Text(message)));
  }

  List<FolderResponse> _subfolders(List<FolderResponse> roots) {
    if (_currentFolder == null) return roots;
    return _currentFolder!.children;
  }

  List<EntryResponse> _entriesInFolder(List<EntryResponse> all) {
    final id = _currentFolder?.id;
    return all.where((e) => e.folderId == id).toList();
  }

  /// Creates a zero-knowledge share link for an entire folder: every entry in it
  /// is decrypted and re-encrypted into a single payload under a random link key
  /// that only ever lives in the URL fragment.
  Future<void> _createFolderShareLink(FolderResponse folder) async {
    final api = ref.read(apiClientProvider);
    final authSvc = ref.read(authServiceProvider);
    if (authSvc.privX25519 == null) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('Vault is locked — unlock first')),
      );
      return;
    }
    final all = ref.read(entriesProvider).valueOrNull ?? const <EntryResponse>[];
    final inFolder = all.where((e) => e.folderId == folder.id).toList();
    if (inFolder.isEmpty) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('This folder has no entries to share')),
      );
      return;
    }
    List<Map<String, dynamic>> items;
    try {
      items = <Map<String, dynamic>>[];
      for (final e in inFolder) {
        final full = await api.getEntry(e.id);
        final encKey = full.entryKey;
        if (encKey == null) continue;
        final dataKey =
            await VaultCrypto.decryptDataKey(encKey.encryptedKey, authSvc.privX25519!, authSvc.privMLKEM!);
        final data = await VaultCrypto.decryptEntryData(
            full.encryptedData, Uint8List.fromList(dataKey));
        items.add({'name': e.name, 'type': e.type, 'url': e.url, 'data': data});
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text('Could not read folder: $e')),
        );
      }
      return;
    }

    if (!mounted) return;
    await showDialog<void>(
      context: context,
      builder: (_) => ShareLinkDialog(
        title: folder.name,
        onCreate: (validity) =>
            _buildFolderShareLink(folder, items, authSvc.privX25519!, validity),
      ),
    );
  }

  /// Encrypts the folder payload under a deterministic link key, creates (or
  /// refreshes) the folder share link, and returns the shareable URL.
  Future<String> _buildFolderShareLink(
    FolderResponse folder,
    List<Map<String, dynamic>> items,
    Uint8List priv,
    Duration? validity,
  ) async {
    final api = ref.read(apiClientProvider);
    final payload = {'folder': folder.name, 'entries': items};
    final linkKey = VaultCrypto.randomKey();
    final encryptedPayload =
        await VaultCrypto.encryptShareLinkPayload(linkKey, payload);
    final exp = validity == null
        ? DateTime.utc(2125)
        : DateTime.now().toUtc().add(validity);
    final expStr = '${exp.toIso8601String().split('.').first}Z';
    final link = await api.createFolderShareLink(
      folder.id,
      CreateShareLinkRequest(
        encryptedPayload: encryptedPayload,
        payloadNonce: base64.encode(Uint8List(12)),
        expiresAt: expStr,
      ),
    );
    ref.invalidate(sharesProvider);
    final secret = base64Url.encode(linkKey);
    return '${api.publicBaseUrl}/web/#/share/${link.token}?k=${Uri.encodeQueryComponent(secret)}';
  }

  // ── Per-entry actions (long-press sheet + edit-mode toolbar share one set) ──

  /// Apple-Mail-style action sheet for a single entry (opened via long-press in
  /// non-edit mode). The same actions are exposed as the edit-mode toolbar.
  Future<void> _showEntryActions(EntryResponse entry) async {
    await showModalBottomSheet<void>(
      context: context,
      builder: (ctx) => SafeArea(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            ListTile(
              leading: const Icon(Icons.lock_outline, color: AppTheme.green),
              title: Text(entry.name,
                  style: const TextStyle(fontWeight: FontWeight.w600)),
              subtitle: entry.url.isNotEmpty
                  ? Text(entry.url, overflow: TextOverflow.ellipsis)
                  : null,
            ),
            const Divider(height: 1),
            ListTile(
              leading: const Icon(Icons.ios_share),
              title: const Text('Share link'),
              onTap: () {
                Navigator.pop(ctx);
                _shareEntries([entry]);
              },
            ),
            ListTile(
              leading: const Icon(Icons.drive_file_move_outline),
              title: const Text('Move to folder'),
              onTap: () {
                Navigator.pop(ctx);
                _moveEntries([entry]);
              },
            ),
            ListTile(
              leading: const Icon(Icons.copy_all_outlined),
              title: const Text('Duplicate'),
              onTap: () {
                Navigator.pop(ctx);
                _duplicateEntries([entry]);
              },
            ),
            ListTile(
              leading: const Icon(Icons.delete_outline, color: AppTheme.error),
              title: const Text('Delete',
                  style: TextStyle(color: AppTheme.error)),
              onTap: () {
                Navigator.pop(ctx);
                _deleteEntries([entry]);
              },
            ),
          ],
        ),
      ),
    );
  }

  /// Creates a zero-knowledge share link for a single entry. Sharing multiple
  /// entries as one link is not supported (each link targets one resource), so
  /// this no-ops with a hint when more than one is selected.
  Future<void> _shareEntries(List<EntryResponse> entries) async {
    if (entries.length != 1) {
      _snack('Select a single entry to share');
      return;
    }
    final entry = entries.first;
    final authSvc = ref.read(authServiceProvider);
    final priv = authSvc.privX25519;
    final privM = authSvc.privMLKEM;
    if (priv == null || privM == null) {
      _snack('Vault is locked — unlock first');
      return;
    }
    Map<String, dynamic> data;
    try {
      final full = await ref.read(apiClientProvider).getEntry(entry.id);
      final encKey = full.entryKey;
      if (encKey == null) throw Exception('No entry key');
      final dataKey =
          await VaultCrypto.decryptDataKey(encKey.encryptedKey, priv, privM);
      data = await VaultCrypto.decryptEntryData(
          full.encryptedData, Uint8List.fromList(dataKey));
    } catch (e) {
      _snack('Could not read entry: $e');
      return;
    }
    if (!mounted) return;
    await showDialog<void>(
      context: context,
      builder: (_) => ShareLinkDialog(
        title: entry.name,
        onCreate: (validity) =>
            _buildEntryShareLink(entry, data, priv, validity),
      ),
    );
  }

  /// Encrypts the entry payload under a deterministic link key, creates (or
  /// refreshes) the entry share link, and returns the shareable URL.
  Future<String> _buildEntryShareLink(
    EntryResponse entry,
    Map<String, dynamic> data,
    Uint8List priv,
    Duration? validity,
  ) async {
    final api = ref.read(apiClientProvider);
    final payload = {
      'name': entry.name,
      'type': entry.type,
      'url': entry.url,
      'data': data,
    };
    final linkKey = VaultCrypto.randomKey();
    final encryptedPayload =
        await VaultCrypto.encryptShareLinkPayload(linkKey, payload);
    final exp = validity == null
        ? DateTime.utc(2125)
        : DateTime.now().toUtc().add(validity);
    final expStr = '${exp.toIso8601String().split('.').first}Z';
    final link = await api.createEntryShareLink(
      entry.id,
      CreateShareLinkRequest(
        encryptedPayload: encryptedPayload,
        payloadNonce: base64.encode(Uint8List(12)),
        expiresAt: expStr,
      ),
    );
    ref.invalidate(sharesProvider);
    final secret = base64Url.encode(linkKey);
    return '${api.publicBaseUrl}/web/#/share/${link.token}?k=${Uri.encodeQueryComponent(secret)}';
  }

  /// Re-parents the given entries to a folder the user picks (root allowed).
  /// The full entry is re-sent unchanged except for `folder_id` so no encrypted
  /// data is lost.
  Future<void> _moveEntries(List<EntryResponse> entries) async {
    final choice = await _pickTargetFolder();
    if (choice == null) return;
    final api = ref.read(apiClientProvider);
    try {
      for (final e in entries) {
        final full = await api.getEntry(e.id);
        await api.updateEntry(
          e.id,
          UpdateEntryRequest(
            folderId: choice.id,
            name: full.name,
            url: full.url,
            encryptedData: full.encryptedData,
            dataNonce: full.dataNonce,
            entryKeys: full.entryKey != null ? [full.entryKey!] : null,
          ),
        );
      }
      ref.invalidate(entriesProvider);
      _exitEditMode();
      _snack('Moved ${_countLabel(entries.length)}');
    } catch (e) {
      _snack('Move failed: $e');
    }
  }

  /// Decrypts each entry and re-encrypts it under a fresh data key as a new
  /// "(copy)" entry in the same folder.
  Future<void> _duplicateEntries(List<EntryResponse> entries) async {
    final authSvc = ref.read(authServiceProvider);
    final priv = authSvc.privX25519;
    final privM = authSvc.privMLKEM;
    if (priv == null || privM == null) {
      _snack('Vault is locked — unlock first');
      return;
    }
    final api = ref.read(apiClientProvider);
    try {
      final myPub = await authSvc.getPubX25519();
      final myPubMlkem = await authSvc.getPubMlkem768();
      final myUserId = await authSvc.getUserId();
      if (myPub == null || myPubMlkem == null) throw Exception('Public key not found');
      for (final e in entries) {
        final full = await api.getEntry(e.id);
        final encKey = full.entryKey;
        if (encKey == null) continue;
        final oldKey =
            await VaultCrypto.decryptDataKey(encKey.encryptedKey, priv, privM);
        final data = await VaultCrypto.decryptEntryData(
            full.encryptedData, Uint8List.fromList(oldKey));
        final enc = await VaultCrypto.encryptEntryData(data);
        final newEncKey = await VaultCrypto.encryptDataKey(enc.dataKey, myPub, myPubMlkem);
        await api.createEntry(
          CreateEntryRequest(
            type: full.type,
            name: '${full.name} (copy)',
            url: full.url,
            encryptedData: enc.encryptedData,
            dataNonce: enc.dataNonce,
            entryKeys: [
              EntryKey(userId: myUserId ?? '', encryptedKey: newEncKey)
            ],
            folderId: full.folderId,
          ),
        );
      }
      ref.invalidate(entriesProvider);
      _exitEditMode();
      _snack('Duplicated ${_countLabel(entries.length)}');
    } catch (e) {
      _snack('Duplicate failed: $e');
    }
  }

  Future<void> _deleteEntries(List<EntryResponse> entries) async {
    final n = entries.length;
    final ok = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: Text(n == 1 ? 'Delete entry?' : 'Delete $n entries?'),
        content: const Text('This cannot be undone.'),
        actions: [
          TextButton(
              onPressed: () => Navigator.pop(ctx, false),
              child: const Text('Cancel')),
          TextButton(
            onPressed: () => Navigator.pop(ctx, true),
            child:
                const Text('Delete', style: TextStyle(color: AppTheme.error)),
          ),
        ],
      ),
    );
    if (ok != true) return;
    final api = ref.read(apiClientProvider);
    try {
      for (final e in entries) {
        await api.deleteEntry(e.id);
      }
      ref.invalidate(entriesProvider);
      _exitEditMode();
      _snack('Deleted ${_countLabel(n)}');
    } catch (e) {
      _snack('Delete failed: $e');
    }
  }

  String _countLabel(int n) => '$n item${n == 1 ? '' : 's'}';

  /// Modal folder picker returning the chosen folder id (null = root) wrapped in
  /// a record, or null when the user cancels.
  Future<({String? id})?> _pickTargetFolder() async {
    final roots =
        ref.read(foldersProvider).valueOrNull ?? const <FolderResponse>[];
    final options = <({String? id, String label})>[
      (id: null, label: '(No folder · root)'),
    ];
    void walk(List<FolderResponse> fs, int depth) {
      for (final f in fs) {
        options.add((id: f.id, label: '${'    ' * depth}${f.name}'));
        walk(f.children, depth + 1);
      }
    }

    walk(roots, 0);
    return showDialog<({String? id})>(
      context: context,
      builder: (ctx) => SimpleDialog(
        title: const Text('Move to'),
        children: [
          for (final o in options)
            SimpleDialogOption(
              onPressed: () => Navigator.pop(ctx, (id: o.id)),
              child: Padding(
                padding: const EdgeInsets.symmetric(vertical: 8),
                child: Row(
                  children: [
                    Icon(
                      o.id == null ? Icons.home_outlined : Icons.folder_outlined,
                      size: 18,
                      color: AppTheme.green,
                    ),
                    const SizedBox(width: 12),
                    Expanded(
                        child: Text(o.label,
                            overflow: TextOverflow.ellipsis)),
                  ],
                ),
              ),
            ),
        ],
      ),
    );
  }

  List<EntryResponse> _searchEntries(List<EntryResponse> all) {
    final q = _searchQuery.toLowerCase();
    return all
        .where((e) =>
            e.name.toLowerCase().contains(q) ||
            e.url.toLowerCase().contains(q))
        .toList();
  }

  @override
  Widget build(BuildContext context) {
    final foldersAsync = ref.watch(foldersProvider);
    final entriesAsync = ref.watch(entriesProvider);
    final allEntries =
        entriesAsync.valueOrNull ?? const <EntryResponse>[];

    final isSearching = _searchQuery.isNotEmpty;

    final title = _editMode
        ? '> ${_selected.length} SELECTED'
        : _currentFolder == null
            ? '> VAULT'
            : '> ${_currentFolder!.name.toUpperCase()}';

    return PopScope(
      canPop: _atRoot && !_editMode,
      onPopInvokedWithResult: (didPop, _) {
        if (didPop) return;
        if (_editMode) {
          _exitEditMode();
        } else if (!_atRoot) {
          setState(() => _stack.removeLast());
        }
      },
      child: Scaffold(
        appBar: AppBar(
          title: Text(title),
          leading: _editMode
              ? IconButton(
                  icon: const Icon(Icons.close),
                  tooltip: 'Done',
                  onPressed: _exitEditMode,
                )
              : _atRoot
                  ? null
                  : IconButton(
                      icon: const Icon(Icons.arrow_back),
                      onPressed: () => setState(() => _stack.removeLast()),
                    ),
          actions: _editMode
              ? [
                  TextButton(
                    onPressed: () => setState(() {
                      final visible =
                          _visibleEntryIds(allEntries, isSearching);
                      if (_selected.containsAll(visible)) {
                        _selected.removeAll(visible);
                      } else {
                        _selected.addAll(visible);
                      }
                    }),
                    child: const Text('All',
                        style: TextStyle(color: AppTheme.green)),
                  ),
                ]
              : [
                  if (_currentFolder != null)
                    IconButton(
                      icon: const Icon(Icons.link_outlined),
                      tooltip: 'Share this folder',
                      onPressed: () => _createFolderShareLink(_currentFolder!),
                    ),
                  IconButton(
                    icon: const Icon(Icons.checklist),
                    tooltip: 'Select',
                    onPressed: () => setState(() => _editMode = true),
                  ),
                  IconButton(
                    icon: const Icon(Icons.settings_outlined),
                    onPressed: () => context.go('/settings'),
                  ),
                ],
          bottom: PreferredSize(
            preferredSize: const Size.fromHeight(56),
            child: Padding(
              padding: const EdgeInsets.fromLTRB(16, 0, 16, 8),
              child: TextField(
                controller: _searchCtrl,
                onChanged: (v) => setState(() => _searchQuery = v),
                decoration: InputDecoration(
                  hintText: 'search entries...',
                  prefixIcon: const Icon(Icons.search, size: 18),
                  suffixIcon: _searchCtrl.text.isNotEmpty
                      ? IconButton(
                          icon: const Icon(Icons.clear, size: 18),
                          onPressed: () {
                            _searchCtrl.clear();
                            setState(() => _searchQuery = '');
                          },
                        )
                      : null,
                  contentPadding:
                      const EdgeInsets.symmetric(vertical: 8, horizontal: 12),
                  isDense: true,
                ),
              ),
            ),
          ),
        ),
        body: foldersAsync.when(
          loading: () => const Center(child: CircularProgressIndicator()),
          error: (e, _) => _ErrorState(error: e.toString()),
          data: (rootFolders) => entriesAsync.when(
            loading: () => const Center(child: CircularProgressIndicator()),
            error: (e, _) => _ErrorState(error: e.toString()),
            data: (allEntries) {
              if (isSearching) {
                final results = _searchEntries(allEntries);
                if (results.isEmpty) {
                  return const _EmptyState(mode: _EmptyMode.search);
                }
                return RefreshIndicator(
                  color: AppTheme.green,
                  onRefresh: () async => ref.invalidate(entriesProvider),
                  child: ListView.separated(
                    itemCount: results.length,
                    separatorBuilder: (_, _) => const Divider(height: 1),
                    itemBuilder: (ctx, i) => _buildEntryTile(results[i]),
                  ),
                );
              }

              final folders = _subfolders(rootFolders);
              final entries = _entriesInFolder(allEntries);

              if (folders.isEmpty && entries.isEmpty) {
                return _EmptyState(
                  mode: _atRoot ? _EmptyMode.vault : _EmptyMode.folder,
                );
              }

              return RefreshIndicator(
                color: AppTheme.green,
                onRefresh: () async {
                  ref.invalidate(foldersProvider);
                  ref.invalidate(entriesProvider);
                },
                child: ListView.separated(
                  itemCount: folders.length + entries.length,
                  separatorBuilder: (_, _) => const Divider(height: 1),
                  itemBuilder: (ctx, i) {
                    if (i < folders.length) {
                      return _FolderTile(
                        folder: folders[i],
                        onTap: () => setState(() {
                          _stack.add(folders[i]);
                          _selected.clear();
                        }),
                      );
                    }
                    return _buildEntryTile(entries[i - folders.length]);
                  },
                ),
              );
            },
          ),
        ),
        floatingActionButton: _editMode
            ? null
            : FloatingActionButton(
                onPressed: () {
                  final fid = _currentFolder?.id;
                  context.go(fid != null
                      ? '/entries/new?folderId=$fid'
                      : '/entries/new');
                },
                child: const Icon(Icons.add),
              ),
        bottomNavigationBar: _editMode
            ? _editToolbar(allEntries)
            : const PbBottomNav(currentIndex: 0),
      ),
    );
  }

  /// Ids of the entries currently visible (search results or current folder),
  /// used by the "All" toggle in the edit-mode app bar.
  Set<String> _visibleEntryIds(List<EntryResponse> all, bool isSearching) {
    final list = isSearching ? _searchEntries(all) : _entriesInFolder(all);
    return list.map((e) => e.id).toSet();
  }

  Widget _buildEntryTile(EntryResponse entry) {
    return _EntryTile(
      entry: entry,
      selectionMode: _editMode,
      selected: _selected.contains(entry.id),
      onTap: () {
        if (_editMode) {
          _toggleSelected(entry.id);
        } else {
          context.go('/entries/${entry.id}');
        }
      },
      onLongPress: _editMode ? null : () => _showEntryActions(entry),
    );
  }

  /// Bottom toolbar shown in edit mode. Buttons are disabled while nothing is
  /// selected; each invokes the same action methods as the long-press sheet.
  Widget _editToolbar(List<EntryResponse> all) {
    final selected =
        all.where((e) => _selected.contains(e.id)).toList(growable: false);
    final has = selected.isNotEmpty;
    return BottomAppBar(
      child: Row(
        mainAxisAlignment: MainAxisAlignment.spaceAround,
        children: [
          _toolButton(Icons.ios_share, 'Share',
              has ? () => _shareEntries(selected) : null),
          _toolButton(Icons.drive_file_move_outline, 'Move',
              has ? () => _moveEntries(selected) : null),
          _toolButton(Icons.copy_all_outlined, 'Duplicate',
              has ? () => _duplicateEntries(selected) : null),
          _toolButton(Icons.delete_outline, 'Delete',
              has ? () => _deleteEntries(selected) : null,
              color: AppTheme.error),
        ],
      ),
    );
  }

  Widget _toolButton(IconData icon, String label, VoidCallback? onTap,
      {Color color = AppTheme.green}) {
    final enabled = onTap != null;
    final c = enabled ? color : AppTheme.onBgDim;
    return InkWell(
      onTap: onTap,
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(icon, size: 22, color: c),
            const SizedBox(height: 2),
            Text(label, style: TextStyle(fontSize: 11, color: c)),
          ],
        ),
      ),
    );
  }
}

class _FolderTile extends StatelessWidget {
  final FolderResponse folder;
  final VoidCallback onTap;
  const _FolderTile({required this.folder, required this.onTap});

  @override
  Widget build(BuildContext context) {
    return ListTile(
      leading: Container(
        width: 36,
        height: 36,
        decoration: BoxDecoration(
          border: Border.all(color: AppTheme.border),
          color: AppTheme.surfaceVariant,
        ),
        child: const Icon(Icons.folder_outlined, size: 18, color: AppTheme.green),
      ),
      title: Text(
        folder.name,
        style: const TextStyle(fontWeight: FontWeight.w500, fontSize: 14),
      ),
      subtitle: folder.children.isNotEmpty
          ? Text(
              '${folder.children.length} subfolder${folder.children.length == 1 ? '' : 's'}',
              style: const TextStyle(color: AppTheme.onBgDim, fontSize: 12),
            )
          : null,
      trailing: const Icon(Icons.chevron_right, size: 18, color: AppTheme.onBgDim),
      onTap: onTap,
    );
  }
}

class _EntryTile extends StatelessWidget {
  final EntryResponse entry;
  final bool selectionMode;
  final bool selected;
  final VoidCallback onTap;
  final VoidCallback? onLongPress;
  const _EntryTile({
    required this.entry,
    required this.onTap,
    this.onLongPress,
    this.selectionMode = false,
    this.selected = false,
  });

  @override
  Widget build(BuildContext context) {
    return ListTile(
      tileColor: selected ? AppTheme.greenFaint : null,
      leading: selectionMode
          ? Icon(
              selected
                  ? Icons.check_circle
                  : Icons.radio_button_unchecked,
              color: selected ? AppTheme.green : AppTheme.onBgDim,
            )
          : _typeIcon(entry.type),
      title: Text(
        entry.name,
        style: const TextStyle(fontWeight: FontWeight.w500, fontSize: 14),
      ),
      subtitle: entry.url.isNotEmpty
          ? Text(
              entry.url,
              style: const TextStyle(color: AppTheme.onBgDim, fontSize: 12),
              overflow: TextOverflow.ellipsis,
            )
          : null,
      trailing: selectionMode
          ? null
          : const Icon(Icons.chevron_right,
              size: 18, color: AppTheme.onBgDim),
      onTap: onTap,
      onLongPress: onLongPress,
    );
  }

  Widget _typeIcon(String type) {
    final (icon, color) = switch (type) {
      'totp' => (Icons.schedule, AppTheme.green),
      'note' => (Icons.note_outlined, Colors.amber),
      'api-key' => (Icons.api_outlined, Colors.purple),
      'ssh-key' => (Icons.terminal_outlined, Colors.cyan),
      _ => (Icons.lock_outline, AppTheme.onBgDim),
    };
    return Container(
      width: 36,
      height: 36,
      decoration: BoxDecoration(
        border: Border.all(color: AppTheme.border),
        color: AppTheme.surfaceVariant,
      ),
      child: Icon(icon, size: 18, color: color),
    );
  }
}

enum _EmptyMode { vault, folder, search }

class _EmptyState extends StatelessWidget {
  final _EmptyMode mode;
  const _EmptyState({required this.mode});

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(
            mode == _EmptyMode.search
                ? Icons.search_off
                : Icons.lock_open_outlined,
            size: 64,
            color: AppTheme.onBgDim,
          ),
          const SizedBox(height: 16),
          Text(
            switch (mode) {
              _EmptyMode.vault => 'Your vault is empty',
              _EmptyMode.folder => 'This folder is empty',
              _EmptyMode.search => 'No entries found',
            },
            style: const TextStyle(color: AppTheme.onBgDim),
          ),
          if (mode == _EmptyMode.vault) ...[
            const SizedBox(height: 16),
            PbButton(
              label: 'Add First Entry',
              onPressed: () => context.go('/entries/new'),
              icon: Icons.add,
            ),
          ],
        ],
      ),
    );
  }
}

class _ErrorState extends StatelessWidget {
  final String error;
  const _ErrorState({required this.error});

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Icon(Icons.error_outline, color: AppTheme.error, size: 48),
            const SizedBox(height: 12),
            Text(error,
                textAlign: TextAlign.center,
                style: const TextStyle(color: AppTheme.error)),
          ],
        ),
      ),
    );
  }
}
