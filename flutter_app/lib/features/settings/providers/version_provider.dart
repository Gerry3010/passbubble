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

import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/api/api_client.dart';
import '../../../core/api/github_client.dart';

class VersionInfo {
  final String serverVersion;
  final String latestVersion;
  final String releaseNotes;
  final String releaseUrl;

  const VersionInfo({
    required this.serverVersion,
    required this.latestVersion,
    required this.releaseNotes,
    required this.releaseUrl,
  });

  bool get isUpToDate => _normalize(serverVersion) == _normalize(latestVersion);

  // Strip leading "v" for comparison: "v2.0.0" == "2.0.0"
  static String _normalize(String v) =>
      v.startsWith('v') ? v.substring(1) : v;
}

final versionInfoProvider = FutureProvider<VersionInfo>((ref) async {
  final api = ref.read(apiClientProvider);
  final github = ref.read(githubClientProvider);

  final results = await Future.wait([
    api.health().catchError((_) => <String, dynamic>{'version': 'unknown'}),
    github.fetchLatestRelease(),
  ]);

  final health = results[0] as Map<String, dynamic>;
  final release = results[1] as GithubRelease;

  return VersionInfo(
    serverVersion: (health['version'] as String?) ?? 'unknown',
    latestVersion: release.tagName,
    releaseNotes: release.body,
    releaseUrl: release.htmlUrl,
  );
});
