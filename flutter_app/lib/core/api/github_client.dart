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

import 'package:dio/dio.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

final githubClientProvider = Provider<GithubClient>((_) => GithubClient());

class GithubRelease {
  final String tagName;
  final String htmlUrl;
  final String body;
  final DateTime publishedAt;

  const GithubRelease({
    required this.tagName,
    required this.htmlUrl,
    required this.body,
    required this.publishedAt,
  });

  factory GithubRelease.fromJson(Map<String, dynamic> json) => GithubRelease(
        tagName: json['tag_name'] as String,
        htmlUrl: json['html_url'] as String,
        body: (json['body'] as String?) ?? '',
        publishedAt: DateTime.parse(json['published_at'] as String),
      );
}

class GithubClient {
  final Dio _dio = Dio(BaseOptions(
    baseUrl: 'https://api.github.com',
    headers: {
      'Accept': 'application/vnd.github+json',
      'X-GitHub-Api-Version': '2022-11-28',
    },
    connectTimeout: const Duration(seconds: 10),
    receiveTimeout: const Duration(seconds: 10),
  ));

  Future<GithubRelease> fetchLatestRelease() async {
    final resp = await _dio.get(
      '/repos/Gerry3010/passbubble/releases/latest',
    );
    return GithubRelease.fromJson(resp.data as Map<String, dynamic>);
  }
}
