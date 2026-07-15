# Passbubble — Claude Code Guidelines

## Repository Layout

This is a Go + Flutter + TypeScript monorepo:

```
backend/        Go REST API server (own go.mod)
cli/            Go TUI/CLI client (own go.mod, depends on backend/ via replace)
flutter_app/    Flutter mobile/web app
packages/shared-ts/   TypeScript E2E crypto library (used by browser extension)
extension/      Chrome + Firefox MV3 browser extension
```

Run tests from the right module directory:
```bash
cd backend && go test ./...
cd cli && go test ./...
```

## License Headers

**Every source file must start with the AGPL v3 header.** Use this exact block:

```go
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
```

Applies to: `.go`, `.ts`, `.tsx` files. The `//` comment style works for both Go and TypeScript.

## Workflow: No Worktrees

**Do not use git worktrees for feature work.** Merging multiple worktrees back into main requires manual conflict resolution for every shared file (Makefile, docker-compose.yml, go.mod, etc.) and is error-prone.

Instead:
- Work directly on `main` or short-lived feature branches
- Create a normal branch: `git checkout -b feature/my-feature`
- Merge via PR or `git merge` when done

## Make Targets

```bash
make up              # Start dev stack (postgres + redis + backend via Docker)
make dev             # Run backend locally (go run)
make test            # backend + CLI tests
make build-all       # Build backend + CLI binaries → build/
make lint            # golangci-lint + flutter analyze
make migrate         # Run DB migrations (needs DB_URL or .env)
make sqlc            # Regenerate sqlc type-safe query code
make build-extension # Build Chrome + Firefox extension
make test-extension  # Run extension + shared-ts tests
```

## Key Conventions

- Backend and CLI are separate Go modules — the CLI references backend via a `replace` directive in `cli/go.mod`
- DB migrations live in `backend/internal/db/migrations/` (golang-migrate format)
- The backend binary embeds the Flutter web app (`backend/internal/static/web/`) and admin panel at build time via `//go:embed`
- Browser extension CSP boundary: all crypto runs in the background service worker, never in content scripts

## Conventional Commits

All commit messages must follow [Conventional Commits](https://www.conventionalcommits.org/):

```
feat:      new feature (bumps minor version)
fix:       bug fix (bumps patch version)
chore:     maintenance, deps, tooling
docs:      documentation only
test:      tests only
refactor:  no behavior change
ci:        CI/CD pipeline changes
```

Breaking change: use `feat!:` / `fix!:` or add a `BREAKING CHANGE:` footer.

## Changelog

`CHANGELOG.md` at the repo root follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) format:

- Keep an `[Unreleased]` section for in-progress work
- On release: rename `[Unreleased]` → `[vX.Y.Z] - YYYY-MM-DD`
- GitHub Releases use the same content (the `release.yml` workflow generates the release notes)

## Pre-Commit / Pre-Tag Checklist

**Always run the full test suite and linter before committing or tagging — no exceptions:**

```bash
# The mailer brand icon is a //go:embed asset generated from assets/svg/icon-extension.svg
# (the single source of truth — never committed; see docs/icons.md). Generate it
# before compiling the backend:
make mailer-icon
cd backend && go build ./... && go vet ./... && go test ./...
cd cli     && go build ./... && go vet ./... && go test -race ./...
cd flutter_app && flutter analyze && flutter test
# Lint (mirrors CI — catches errcheck, staticcheck, etc.)
cd backend && golangci-lint run ./...
cd cli     && golangci-lint run ./...
```

Failures in any of these block the commit/tag. CI mirrors these exact checks — a local green run prevents wasted pipeline cycles.

## Version Management

The backend version is injected at build time via Go ldflags into `backend/internal/version/version.go`:

```
-X github.com/Gerry3010/passbubble/backend/internal/version.Version=$(VERSION)
```

The `VERSION` variable in the Makefile defaults to `git describe --tags --always --dirty`.

**To cut a release:** tag with `vX.Y.Z` → GH Actions `release.yml` handles everything automatically:
tests → CLI binaries → Flutter apps → Docker image (DockerHub: `gerre01/passbubble-server`) → GitHub Release.

The Flutter app's **Version & Updates** screen (Settings → Check for updates) fetches the server version from
`GET /health` and the latest release from the GitHub Releases API, then shows the update command.

## Deploying to the test server (local, gitignored)

For quickly testing the **current working tree** on the live server — without pushing or going
through DockerHub — there is a **gitignored** `deploy.local.mk` providing a `make deploy` target
(wired into the Makefile via `-include deploy.local.mk`; the leading `-` makes it a no-op for CI
and fresh clones). It builds the image, ships it over SSH, and recreates the backend container:

```bash
make deploy          # build amd64 image → docker save/scp → load + recreate backend → verify
make deploy-build    # just build the image locally (linux/amd64)
make deploy-ship     # docker save | gzip + scp to the server
make deploy-restart  # docker load + `docker compose up -d --force-recreate backend` on the server
make deploy-verify   # wait for healthy, print /health version + last migration log line
make deploy-logs     # tail backend logs on the server
```

How it works:
- The image **embeds the Flutter web app and the DB migrations** (`go:embed`), so a single image
  ships everything. **Migrations run automatically on container start** (`db.Migrate` in
  `cmd/server/main.go`) — no separate migrate step needed.
- The server runs `docker compose` in `/opt/passbubble` against `gerre01/passbubble-server:latest`
  (services `postgres` / `redis` / `backend`). `docker load` replaces the local `:latest` image,
  then `--force-recreate backend` picks it up. Postgres data persists in its volume.
- **No new env vars** were introduced by share-links / jobs / account-2FA — 2FA reuses `JWT_SECRET`
  (pending token + at-rest TOTP-secret key) and the existing `SMTP_*` / `APP_BASE_URL` for the 2FA
  recovery email. Just ensure SMTP stays configured for recovery mails to send.

`deploy.local.mk` holds the server host/dir (`DEPLOY_HOST`, `DEPLOY_DIR`) and is **kept out of
git**. If it's missing, recreate it from the variables above (host, `/opt/passbubble`, image
`gerre01/passbubble-server:latest`, service `backend`).

> This is a fast test-deploy path. The canonical release path is still tagging `vX.Y.Z`
> (`release.yml` → DockerHub).

## Mac-Claude brief 2026-07-03 (full-manager push — test & ship)

`main` now contains seven merged feature phases (SPA save-fix, TOTP autofill,
card/identity autofill + app wallet tab, SSO memory, trash/favorites/history,
password health + HIBP, deeper search). All suites green on Linux (Go, vitest,
flutter test, golangci-lint). Your jobs, in priority order — branch + PR as usual:

1. **Ship a fresh TestFlight build NOW.** Gerry's installed iOS build predates
   the existing search UI and the AutoFill credential provider. Bump the build
   number, archive, upload, then verify on device: app-list search finds
   entries by email/notes/card last-4 (new `searchIndexProvider`), and
   Settings → Passwords → AutoFill shows Passbubble with a working search.
   The autofill sync payload now includes `type` and no longer drops
   password-only entries — nothing on the native side *requires* changes, but
   `CredentialProviderViewController` may optionally use `type` for ranking.
2. **Rebuild the Safari wrapper** — `manifest.safari.json` gained
   `clipboardWrite` and `webNavigation`; the extension bundle has new
   fill-iframe modes (TOTP, card/identity, SSO badge) and a health tab.
   Re-run `safari-web-extension-converter` / rsync per your codemagic setup and
   functionally test in Safari: save bar on a first click (SPA fix), TOTP
   copy flash (openPopup limits!), card fill on a checkout form.
   Note: `webRequest.onAuthRequired` (basic auth) stays a Safari no-op; the
   SSO memory needs `webNavigation` — verify Safari grants it, else the badge
   simply never appears (graceful).
3. **Verify the new app screens on device:** Wallet tab (5-tab bottom nav!),
   Settings → Trash (restore/purge), entry detail → favorite star + history
   sheet (decrypt + restore), Settings → Password health (run with HIBP on —
   check only `api.pwnedpasswords.com/range/<5 chars>` requests leave).
4. Migration 000007 runs automatically on server start; no new env vars.

## Apple-slice follow-ups (macOS-device / iOS Builder)

Open work from the iOS/macOS/Safari push. The macOS Claude owns these — branch + PR.
State at hand-off: iOS app shipped to TestFlight (v2.5.0 build 3); PRs open: macOS
`network.client` and the Safari wrapper.

### iOS AutoFill (core PW + TOTP fill works; UX polish parked)
- **Cache not refreshed on entry change.** `AuthService.refreshAutofill()` is wired after
  create/update/delete, but a changed entry (e.g. URL) doesn't visibly reach the extension.
  Diagnose with `flutter run` attached + temp logging in `_syncAutofill` (does it fire? what
  creds/identities does it write?). Likely the `ASCredentialIdentityStore` update or the
  keychain re-write isn't taking effect mid-session.
- **QuickType "browse / key" affordance opens nothing.** The extension is a separate process —
  read its logs via **Console.app** (filter `PassbubbleAutofill` / `pkd` / `AuthenticationServices`)
  while reproducing; `libimobiledevice` can't see the device here.

### macOS app
- **Native hybrid-KEM linking is on `main`** (commit `7301743`, restored 2026-07-07). The
  `native/build.sh macos` target (universal arm64+x86_64 c-archive → `macos/native/`, gitignored)
  and the Runner `OTHER_LDFLAGS = -force_load … -Wl,-export_dynamic` on all three configs are in
  place, so `DynamicLibrary.process()` can resolve the `pb_*` symbols. (Originally reviewed in the
  now-closed PR #7 against the retired `staging` branch — never merged, hence re-applied.)
  **Remaining on a Mac:** run `./native/build.sh macos` before `flutter build macos`, then do the
  **functional decrypt test** (vault login → open an entry → confirm it decrypts). `network.client`
  entitlement already present.
- Configure macOS signing / distribution (team + App Store record) if shipping to the Mac App Store.

### Safari Web Extension (`safari/Passbubble/`, generated via `safari-web-extension-converter`)
- Bundle ids are `net.geraldhofbauer.passbubble.safari` (+ `.safari.Extension`) — **distinct**
  from the Flutter app (the original `codemagic.yaml` used the colliding `net.geraldhofbauer.passbubble`).
  The Safari PR fixes `codemagic.yaml` (bundle ids, real project path `safari/Passbubble/Passbubble.xcodeproj`,
  resource rsync) — **make sure that lands on `main`** (the Linux-side `codemagic.yaml` still has the
  old ids + path).
- Only the **macOS no-sign build** is verified. Still to do: iOS archive + signing, ASC records for the
  `.safari` ids, and a **functional test** (run macOS app → enable in Safari → Settings → Extensions).
- Decide: keep the copied web `Resources/` committed (snapshot; codemagic rsyncs fresh) vs gitignore +
  build-on-CI.

### iOS housekeeping
- Replace the default **Launch image** (App Store flags the placeholder).
- **`UIScene` lifecycle migration** (Flutter warns it'll become required).
- `NSLocationWhenInUseUsageDescription` warning comes from `file_picker` → `DKImagePickerController`
  (`DKAsset.location: CLLocation?`); left unset deliberately (unused) to avoid App Review questions.

### Export compliance (encryption) — SETTLED, do not re-answer wrong
- `ITSAppUsesNonExemptEncryption` = **`true`** is now committed in both
  `ios/Runner/Info.plist` and `macos/Runner/Info.plist` → the per-build Xcode
  export-compliance dialog no longer appears. **Correct value: `true`, NOT `false`.**
  Passbubble implements its own E2E crypto (AES-256, X25519, HKDF, ML-KEM/Kyber
  FIPS 203) beyond OS APIs/HTTPS/Keychain, so it does *not* qualify for the simple
  "exempt" cases. Standard algorithms → **not** proprietary (Xcode dialog: pick
  option 2 "standard algorithms in addition to Apple's OS encryption", never 1/3/4).
- **On the FIRST TestFlight/App Store upload**, App Store Connect asks a one-time
  follow-up: *"Does your app qualify for any of the exemptions in Category 5, Part 2
  of the EAR?"* → answer **YES** (mass-market, ECCN **5D992.c**). ASC then remembers
  it per app; no CCATS upload needed.
- **Obligation that remains (paperwork, not code):** an **annual self-classification
  report** (CSV) to **crypt-supp8@bis.doc.gov** (BIS) *and* **enc@nsa.gov** (NSA),
  due **Feb 1** for the prior calendar year — but only once the app was actually
  distributed via App Store/TestFlight in that year. If nothing changed vs. last
  year, a one-line "no changes" email suffices.
- France question in the questionnaire was answered **No** (separate French import
  declaration). Fine to proceed; revisit only if a French-market ANSSI declaration
  is ever required (standard mass-market crypto is normally simplified/exempt there).
