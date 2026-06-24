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

import { ApiError } from './errors.js';
import type {
  CreateEntryRequest,
  EntryResponse,
  FolderResponse,
  GenerateRequest,
  GenerateResponse,
  LoginRequest,
  LoginResponse,
  MeResponse,
  RefreshResponse,
  UpdateEntryRequest,
  UserPublicKeys,
  UserSearchResult,
} from '../types/api.js';

export class PassbubbleClient {
  private accessToken?: string;
  private refreshToken?: string;
  private accessTokenExpiresAt = 0;
  private refreshPromise: Promise<void> | null = null;

  constructor(public baseUrl: string) {}

  setTokens(accessToken: string, refreshToken: string, expiresIn: number): void {
    this.accessToken = accessToken;
    this.refreshToken = refreshToken;
    this.accessTokenExpiresAt = Date.now() + expiresIn * 1000;
  }

  clearTokens(): void {
    this.accessToken = undefined;
    this.refreshToken = undefined;
    this.accessTokenExpiresAt = 0;
  }

  isTokenExpiringSoon(): boolean {
    return Date.now() > this.accessTokenExpiresAt - 60_000;
  }

  private async ensureToken(): Promise<void> {
    if (!this.accessToken) return;
    if (!this.isTokenExpiringSoon()) return;
    if (!this.refreshToken) return;
    // Deduplicate concurrent refresh calls
    if (!this.refreshPromise) {
      this.refreshPromise = this.refresh(this.refreshToken)
        .then((r) => {
          this.setTokens(r.access_token, r.refresh_token, r.expires_in);
        })
        .finally(() => {
          this.refreshPromise = null;
        });
    }
    await this.refreshPromise;
  }

  private async request<T>(
    method: string,
    path: string,
    body?: unknown,
    skipAuth = false,
  ): Promise<T> {
    if (!skipAuth) await this.ensureToken();

    const headers: Record<string, string> = { 'Content-Type': 'application/json' };
    if (!skipAuth && this.accessToken) {
      headers['Authorization'] = `Bearer ${this.accessToken}`;
    }

    const res = await fetch(`${this.baseUrl}${path}`, {
      method,
      headers,
      body: body != null ? JSON.stringify(body) : undefined,
    });

    if (!res.ok) {
      let msg = res.statusText;
      try {
        const data = (await res.json()) as { error?: string };
        if (data.error) msg = data.error;
      } catch {
        // ignore
      }
      throw new ApiError(res.status, msg);
    }

    if (res.status === 204) return undefined as T;
    return res.json() as Promise<T>;
  }

  async login(email: string, password: string): Promise<LoginResponse> {
    const body: LoginRequest = { email, password };
    return this.request<LoginResponse>('POST', '/api/v1/auth/login', body, true);
  }

  /** Completes the second step of a 2FA login, returning the full session. */
  async verifyTotp(pendingToken: string, code: string): Promise<LoginResponse> {
    return this.request<LoginResponse>(
      'POST',
      '/api/v1/auth/verify-totp',
      { pending_token: pendingToken, code },
      true,
    );
  }

  async refresh(refreshToken: string): Promise<RefreshResponse> {
    return this.request<RefreshResponse>(
      'POST',
      '/api/v1/auth/refresh',
      { refresh_token: refreshToken },
      true,
    );
  }

  async logout(refreshToken: string): Promise<void> {
    await this.request<void>('POST', '/api/v1/auth/logout', { refresh_token: refreshToken });
  }

  async me(): Promise<MeResponse> {
    return this.request<MeResponse>('GET', '/api/v1/auth/me');
  }

  async listEntries(): Promise<EntryResponse[]> {
    return this.request<EntryResponse[]>('GET', '/api/v1/entries');
  }

  /** Like listEntries() but the entries include encrypted_data and the caller's
   * entry_key, so the client can decrypt fields (e.g. usernames) in bulk. */
  async listEntriesFull(): Promise<EntryResponse[]> {
    return this.request<EntryResponse[]>('GET', '/api/v1/entries/full');
  }

  async getEntry(id: string): Promise<EntryResponse> {
    return this.request<EntryResponse>('GET', `/api/v1/entries/${id}`);
  }

  async searchEntries(q: string): Promise<EntryResponse[]> {
    return this.request<EntryResponse[]>('GET', `/api/v1/entries/search?q=${encodeURIComponent(q)}`);
  }

  async createEntry(req: CreateEntryRequest): Promise<{ id: string }> {
    return this.request<{ id: string }>('POST', '/api/v1/entries', req);
  }

  async updateEntry(id: string, req: UpdateEntryRequest): Promise<void> {
    return this.request<void>('PUT', `/api/v1/entries/${id}`, req);
  }

  async deleteEntry(id: string): Promise<void> {
    return this.request<void>('DELETE', `/api/v1/entries/${id}`);
  }

  async listFolders(): Promise<FolderResponse[]> {
    return this.request<FolderResponse[]>('GET', '/api/v1/folders');
  }

  async getUserKeys(userId: string): Promise<UserPublicKeys> {
    return this.request<UserPublicKeys>('GET', `/api/v1/users/${userId}/keys`);
  }

  async searchUsers(q: string): Promise<UserSearchResult[]> {
    return this.request<UserSearchResult[]>(
      'GET',
      `/api/v1/users/search?q=${encodeURIComponent(q)}`,
    );
  }

  async generate(req: GenerateRequest): Promise<GenerateResponse> {
    return this.request<GenerateResponse>('POST', '/api/v1/generate', req);
  }

  async healthCheck(): Promise<boolean> {
    try {
      const res = await fetch(`${this.baseUrl}/health`);
      return res.ok;
    } catch {
      return false;
    }
  }
}
