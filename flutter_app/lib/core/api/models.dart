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

// API request/response models mirroring the Go backend.

class LoginResponse {
  final String accessToken;
  final String refreshToken;
  final int expiresIn;
  final String userId;
  final String role;
  final String email;
  final String name;
  final String pubX25519;
  final String pubMlkem768;
  final String encPrivX25519;
  final String encPrivMlkem768;
  final String kdfSalt;
  final int kdfTime;
  final int kdfMemory;

  const LoginResponse({
    required this.accessToken,
    required this.refreshToken,
    required this.expiresIn,
    required this.userId,
    required this.role,
    required this.email,
    required this.name,
    required this.pubX25519,
    required this.pubMlkem768,
    required this.encPrivX25519,
    required this.encPrivMlkem768,
    required this.kdfSalt,
    required this.kdfTime,
    required this.kdfMemory,
  });

  factory LoginResponse.fromJson(Map<String, dynamic> j) => LoginResponse(
        accessToken: j['access_token'] as String? ?? '',
        refreshToken: j['refresh_token'] as String? ?? '',
        expiresIn: j['expires_in'] as int? ?? 900,
        userId: j['user_id'] as String? ?? '',
        role: j['role'] as String? ?? 'user',
        email: j['email'] as String? ?? '',
        name: j['name'] as String? ?? '',
        pubX25519: j['pub_x25519'] as String? ?? '',
        pubMlkem768: j['pub_mlkem768'] as String? ?? '',
        encPrivX25519: j['enc_priv_x25519'] as String? ?? '',
        encPrivMlkem768: j['enc_priv_mlkem768'] as String? ?? '',
        kdfSalt: j['kdf_salt'] as String? ?? '',
        kdfTime: j['kdf_time'] as int? ?? 3,
        kdfMemory: j['kdf_memory'] as int? ?? 65536,
      );
}

class RefreshResponse {
  final String accessToken;
  final String refreshToken;
  final int expiresIn;
  const RefreshResponse({
    required this.accessToken,
    required this.refreshToken,
    required this.expiresIn,
  });
  factory RefreshResponse.fromJson(Map<String, dynamic> j) => RefreshResponse(
        accessToken: j['access_token'] as String,
        refreshToken: j['refresh_token'] as String,
        expiresIn: j['expires_in'] as int? ?? 900,
      );
}

class RegisterRequest {
  final String email;
  final String name;
  final String password;
  final String invitationToken;
  final String pubX25519;
  final String pubMlkem768;
  final String encPrivX25519;
  final String encPrivMlkem768;
  final String kdfSalt;

  const RegisterRequest({
    required this.email,
    required this.name,
    required this.password,
    required this.invitationToken,
    required this.pubX25519,
    required this.pubMlkem768,
    required this.encPrivX25519,
    required this.encPrivMlkem768,
    required this.kdfSalt,
  });

  Map<String, dynamic> toJson() => {
        'email': email,
        'name': name,
        'password': password,
        'invitation_token': invitationToken,
        'pub_x25519': pubX25519,
        'pub_mlkem768': pubMlkem768,
        'enc_priv_x25519': encPrivX25519,
        'enc_priv_mlkem768': encPrivMlkem768,
        'kdf_salt': kdfSalt,
      };
}

class UserResponse {
  final String id;
  final String email;
  final String name;
  final String role;
  final String? status;
  const UserResponse({
    required this.id,
    required this.email,
    required this.name,
    required this.role,
    this.status,
  });
  factory UserResponse.fromJson(Map<String, dynamic> j) => UserResponse(
        id: j['id'] as String,
        email: j['email'] as String,
        name: j['name'] as String,
        role: j['role'] as String? ?? 'user',
        status: j['status'] as String?,
      );
}

class UserPublicKeys {
  final String userId;
  final String pubX25519;
  final String pubMlkem768;
  const UserPublicKeys({
    required this.userId,
    required this.pubX25519,
    required this.pubMlkem768,
  });
  factory UserPublicKeys.fromJson(Map<String, dynamic> j) => UserPublicKeys(
        userId: j['user_id'] as String,
        pubX25519: j['pub_x25519'] as String,
        pubMlkem768: j['pub_mlkem768'] as String,
      );
}

class FolderResponse {
  final String id;
  final String name;
  final String? parentId;
  final List<FolderResponse> children;
  const FolderResponse({
    required this.id,
    required this.name,
    this.parentId,
    this.children = const [],
  });
  factory FolderResponse.fromJson(Map<String, dynamic> j) => FolderResponse(
        id: j['id'] as String,
        name: j['name'] as String,
        parentId: j['parent_id'] as String?,
        children: (j['children'] as List?)
                ?.map((c) => FolderResponse.fromJson(c as Map<String, dynamic>))
                .toList() ??
            [],
      );
}

class EntryKey {
  final String userId;
  final String encryptedKey;
  const EntryKey({required this.userId, required this.encryptedKey});
  Map<String, dynamic> toJson() => {
        'user_id': userId,
        'encrypted_key': encryptedKey,
      };
  factory EntryKey.fromJson(Map<String, dynamic> j) => EntryKey(
        userId: j['user_id'] as String,
        encryptedKey: j['encrypted_key'] as String,
      );
}

class CreateEntryRequest {
  final String? folderId;
  final String type;
  final String name;
  final String? url;
  final String encryptedData;
  final String dataNonce;
  final List<EntryKey> entryKeys;
  const CreateEntryRequest({
    this.folderId,
    required this.type,
    required this.name,
    this.url,
    required this.encryptedData,
    required this.dataNonce,
    required this.entryKeys,
  });
  Map<String, dynamic> toJson() => {
        if (folderId != null) 'folder_id': folderId,
        'type': type,
        'name': name,
        if (url != null && url!.isNotEmpty) 'url': url,
        'encrypted_data': encryptedData,
        'data_nonce': dataNonce,
        'entry_keys': entryKeys.map((k) => k.toJson()).toList(),
      };
}

class UpdateEntryRequest {
  final String? name;
  final String? url;
  final String? encryptedData;
  final String? dataNonce;
  final List<EntryKey>? entryKeys;
  const UpdateEntryRequest({
    this.name,
    this.url,
    this.encryptedData,
    this.dataNonce,
    this.entryKeys,
  });
  Map<String, dynamic> toJson() => {
        if (name != null) 'name': name,
        if (url != null) 'url': url,
        if (encryptedData != null) 'encrypted_data': encryptedData,
        if (dataNonce != null) 'data_nonce': dataNonce,
        if (entryKeys != null) 'entry_keys': entryKeys!.map((k) => k.toJson()).toList(),
      };
}

class EntryResponse {
  final String id;
  final String? folderId;
  final String type;
  final String name;
  final String url;
  final String encryptedData;
  final String dataNonce;
  final EntryKey? entryKey;
  final String permission;
  final String createdAt;
  final String updatedAt;
  const EntryResponse({
    required this.id,
    this.folderId,
    required this.type,
    required this.name,
    required this.url,
    required this.encryptedData,
    required this.dataNonce,
    this.entryKey,
    required this.permission,
    required this.createdAt,
    required this.updatedAt,
  });
  factory EntryResponse.fromJson(Map<String, dynamic> j) => EntryResponse(
        id: j['id'] as String,
        folderId: j['folder_id'] as String?,
        type: j['type'] as String? ?? 'password',
        name: j['name'] as String,
        url: j['url'] as String? ?? '',
        encryptedData: j['encrypted_data'] as String? ?? '',
        dataNonce: j['data_nonce'] as String? ?? '',
        entryKey: j['entry_key'] != null
            ? EntryKey.fromJson(j['entry_key'] as Map<String, dynamic>)
            : null,
        permission: j['permission'] as String? ?? 'read',
        createdAt: j['created_at'] as String? ?? '',
        updatedAt: j['updated_at'] as String? ?? '',
      );
}

class ShareEntryRequest {
  final String userId;
  final String permission;
  final String encryptedKey;
  const ShareEntryRequest({
    required this.userId,
    required this.permission,
    required this.encryptedKey,
  });
  Map<String, dynamic> toJson() => {
        'user_id': userId,
        'permission': permission,
        'encrypted_key': encryptedKey,
      };
}

class GenerateResponse {
  final List<GeneratedPassword> passwords;
  const GenerateResponse({required this.passwords});
  factory GenerateResponse.fromJson(Map<String, dynamic> j) => GenerateResponse(
        passwords: (j['passwords'] as List)
            .map((p) => GeneratedPassword.fromJson(p as Map<String, dynamic>))
            .toList(),
      );
}

class GeneratedPassword {
  final String password;
  final int strength;
  const GeneratedPassword({required this.password, required this.strength});
  factory GeneratedPassword.fromJson(Map<String, dynamic> j) =>
      GeneratedPassword(
        password: j['password'] as String,
        strength: j['strength'] as int? ?? 0,
      );
}

class InvitationResponse {
  final String id;
  final String email;
  final String token;
  final bool used;
  const InvitationResponse({
    required this.id,
    required this.email,
    required this.token,
    required this.used,
  });
  factory InvitationResponse.fromJson(Map<String, dynamic> j) =>
      InvitationResponse(
        id: j['id'] as String,
        email: j['email'] as String,
        token: j['token'] as String? ?? '',
        used: j['accepted_at'] != null,
      );
}
