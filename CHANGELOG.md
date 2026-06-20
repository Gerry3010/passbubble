# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [2.3.0] - 2026-06-20

### Added
- **Match-Patterns direkt im Popup verwalten:** jede Eintragszeile hat neben „Details" einen **„+ Site"-Button**, der den **exakten** Host der aktuellen Seite zu den Autofill-Match-Patterns hinzufügt bzw. wieder entfernt (`✓ host`, kein Wildcard-Handling beim Add/Remove). Vorhandene Patterns lassen sich über einen **aufklappbaren „N match sites"-Bereich** anzeigen und einzeln (×) löschen. Die **Popup-Suche matcht jetzt auch Match-Patterns** — inkl. Wildcards (z. B. findet die Suche nach `gist.github.com` einen Eintrag mit `*.github.com`)
- **Autofill-URL-Matching wie Psono + Import der Match-Patterns:** Einträge haben jetzt ein eigenes, unverschlüsseltes Feld `match_patterns` (eigene DB-Spalte, durch alle Clients gereicht: Backend, shared-ts, CLI, Extension) für **mehrere Autofill-URL-Muster pro Eintrag**. Gematcht wird im Extension-Hintergrund ohne Entschlüsselung, mit **Host- und `*`-Wildcard-Mustern** (`github.com` → inkl. Subdomains, `*.github.com` → nur Subdomains, `host:port` → Port wird ignoriert); fehlt ein Muster, fällt der Matcher wie bisher auf das `url`-Feld zurück. **Psono-Import** übernimmt die `urlfilter`/`*_url_filter`-Werte als Match-Patterns — sowohl in der **CLI** (`pwmgr import -f psono`) als auch über eine neue **Import-Sektion auf der Options-Seite der Extension** (Psono-JSON hochladen, Einträge werden client-seitig verschlüsselt angelegt)

### Fixed
- **Autofill funktioniert jetzt überhaupt** — vorher tat sich auf Login-Seiten nichts. Mehrere Ursachen behoben:
  - **iframe-Logins** (Apple/SSO/Banken): das Content-Script lief nur im Top-Frame, das Passwortfeld steckt aber oft in einem cross-origin iframe (`idmsa.apple.com`). Jetzt `all_frames: true` → es läuft auch dort (der iframe-Host matcht den Eintrag per Subdomain-Regel).
  - **postMessage-Origin**: das im Seiten-Kontext laufende Content-Script schickte `FILL_MATCHES` mit der Seiten-Origin; das Fill-Iframe verwarf das (erwartete Extension-Origin) und blieb bei „No matching entries". Validierung jetzt über `event.source === window.parent` (Secrets weiterhin nur aus dem Background mit Session-Check).
  - **fokus-getriebenes Einblenden** statt präsenz-/mutationsgetrieben: die Box erscheint beim Fokussieren eines Login-Felds, verschwindet nach dem Ausfüllen / bei Klick daneben / mit Esc und blitzt nicht mehr in einer Endlosschleife auf.
  - **Terminal-Theme + Auto-Höhe**: das Fill-Fenster ist jetzt im App-Look (statt hell) und schrumpft auf Inhaltshöhe (kein weißer Kasten mehr darunter).
  - Suchfeld-Prefill und „+ Site" nutzen den **Host des Login-Frames** (also den iframe-Host, nicht die Top-Seite).
- **Weißer Hintergrund im Popup** behoben (`html`-Element hatte keine Hintergrundfarbe; nur `body` war dunkel).
- **Browser-Extension konnte von der Flutter-App erstellte Einträge nicht entschlüsseln** („Failed to load entry: hybrid-kem: encrypted key too short", und Autofill blieb leer). Die Flutter-App speichert Entry-Keys im **Legacy-X25519-only-Format** (`ephPub(32) || AES-GCM`, ~92 B), das die Go-Clients (CLI/Backend) per Fallback lesen — die TS-Krypto der Extension kannte aber **nur** das Hybrid-Format (X25519+ML-KEM, ~1180 B). `decryptDataKey` erkennt jetzt beide Formate (gespiegelt von `cli/internal/crypto`), sodass die Extension App-erstellte Einträge öffnen und ausfüllen kann
- **Browser-Extension: `Extension context invalidated`-Fehlerflut** (u. a. im Admin-Panel) behoben. Der `MutationObserver` des Content-Scripts feuerte nach einem Extension-Reload/-Update weiter `sendMessage` in einen toten Runtime und spammte bei DOM-intensiven Seiten den Service-Worker zu. Nachrichten laufen jetzt durch einen abgesicherten `safeSend`, der Observer/Listener abbaut, sobald der Kontext weg ist; der Observer ist zusätzlich **entprellt (300 ms)**

## [2.2.0] - 2026-06-20

### Fixed
- **Browser-Extension: Build, Einträge & lange URLs:** das Popup zeigte „No entries found", weil es den Vault mit leerem Such-Query lud (Backend liefert dafür bewusst `[]`) — geladen wird jetzt die volle Liste, gefiltert wird client-seitig. Lange URLs/Namen in der Liste werden **einzeilig mit „…" abgeschnitten** (Tooltip zeigt den vollen Wert) statt überzulaufen. Außerdem landeten die HTML-Entry-Points im Build unter `dist/<browser>/src/…` statt am Manifest-Pfad → die Extension ließ sich nicht laden; der Post-Build verschiebt sie jetzt korrekt
- **Browser-Extension: Content-Script lief gar nicht** (`Uncaught SyntaxError: Cannot use import statement outside a module`). MV3 injiziert `content_scripts` als **klassische** Scripts, Vite baute es aber als ES-Modul mit Shared-Chunks → der `import` in Zeile 1 warf sofort und Autofill/Save-Detection/Fill-Iframe waren komplett tot. Das Content-Script wird jetzt in einem **eigenen Vite-Build als selbst-enthaltenes IIFE** (`format: 'iife'`, `inlineDynamicImports`) gebaut, ohne `import`-Statements; Popup/Options/Background bleiben ESM (`type: module`)
- **Invitations im Admin-Panel widerrufbar/löschbar:** jede Zeile in der Invitations-Liste hat jetzt einen **REVOKE**-Button (mit Bestätigung), der die Einladung über den neuen Endpoint `DELETE /api/v1/admin/invitations/{id}` endgültig entfernt — der zugehörige Token kann danach nicht mehr eingelöst werden
- **Einladungs-E-Mails werden jetzt tatsächlich versendet:** der Admin-`POST /admin/invite`-Endpoint legte bisher nur die Invitation an und gab den Token zurück (manuelles Kopieren im Admin-Panel) — es ging **nie eine E-Mail raus**. Jetzt verschickt er (wenn SMTP konfiguriert ist) eine Einladungs-Mail mit Registrierungs-Link `…/web/#/register?token=…&email=…`; der Token bleibt als Fallback im Admin-Panel kopierbar, falls der Versand fehlschlägt
- **Registrierung per Einladungs-Link auto-befüllt:** der Register-Screen liest **Token und E-Mail aus den Query-Params** des Einladungs-Links und füllt die Felder vor; danach wird der Token **aus der Browser-URL entfernt** (History-API, nur Web), damit er nicht in Adressleiste/Verlauf/Lesezeichen hängen bleibt. Der Deep-Link überlebt zusätzlich den Start (wie die Share-Links)
- **Share-Popup überarbeitet:** Ablaufzeit wählbar (1 Tag / 7 / 30 / 90 Tage / 1 Jahr / **Never**), und der Link erscheint in einem **Terminal-Style-Feld** (grün auf schwarz, monospace) mit Copy-Button
- **Öffentliche Share-Links öffnen jetzt zuverlässig beim ersten Aufruf** (vorher: erst beim zweiten). Zwei Ursachen: (1) der Router wurde bei jeder Auth-Änderung neu erzeugt → jetzt eine stabile Instanz mit `refreshListenable`; (2) der **Splash-`MaterialApp` überschrieb auf Web das URL-Fragment auf `#/`** und zerstörte so den `/share/...`-Deep-Link, bevor der Router gebaut war. Die App initialisiert jetzt **vor** dem ersten `MaterialApp` (kein Splash-`MaterialApp` mehr) und schnappt sich die Start-URL ganz früh in `main()`, sodass der Deep-Link intakt bleibt
- **Einträge verschieben / Ordner beim Bearbeiten** — der Add/Edit-Screen hat jetzt einen **Ordner-Picker**; beim Speichern bleibt der Eintrag in seinem Ordner (vorher landete er beim Bearbeiten immer im Root) und lässt sich darüber verschieben
- **Share-Links:** der Share-Button im Eintrag-Detail ist jetzt **immer sichtbar** (entschlüsselt bei Bedarf automatisch); die **URL enthält den Host** (vorher fehlte er bei der Web-App); öffentliche Links **öffnen im Inkognito ohne Login** (Deep-Link überlebt den Splash); der **Eintragsname** erscheint in der Share-Liste
- **Mehrere Share-Links pro Eintrag/Ordner** wieder möglich: jeder „Create link" erzeugt einen **eigenständigen** Link mit eigenem Token, Ablauf und Zufallsschlüssel (z. B. 2× 7 Tage + 1× unbegrenzt nebeneinander). Vorher überschrieb die Server-Dedup-Logik einen bestehenden Link bei jedem Teilen — wodurch u. a. ein unbegrenzter Link verschwand, sobald man für denselben Eintrag einen kürzeren erstellte
- Flutter-App: **kein fälschliches „Session expired" mehr nach einem Server-Neustart**. Mehrere gleichzeitige Requests beim Resume lösten einen Refresh-Stampede aus (alle erneuerten mit demselben rotierenden Refresh-Token; die Verlierer löschten die frisch rotierte Session → Logout). Jetzt: **Single-Flight-Refresh** (nur ein Refresh gleichzeitig) und Logout **nur bei echter Token-Ablehnung** (401 vom Refresh-Endpoint); transiente Fehler (Neustart/Netz) lassen die Session bestehen und zeigen einen wiederholbaren Verbindungsfehler

### Added
- **Browser-Extension im Terminal-Look + Ordner-Browser:** Popup und Options-Seite teilen jetzt das durchgängige Terminal-Theme (Phosphor-Grün auf Schwarz, Monospace, eigenes transparentes, größer skaliertes Extension-Icon). Das Vault-Popup zeigt beim Öffnen **Ordner statt einer flachen Liste** und **sucht automatisch nach dem Host der aktuellen Seite** (Suchfeld vorbelegt, leeren → Ordner durchstöbern). Neuer **Logout** (Account-Bereich der Options-Seite, per Klick auf den `[Namen]` im Popup erreichbar) löscht Refresh-Token + Session vollständig — abgegrenzt vom reinen Lock. **In-Progress-Login bleibt erhalten**, wenn das Popup zwischendurch schließt (E-Mail/Passwort-Entwurf und 2FA-Schritt via `storage.session`)
- **Transaktions-E-Mails im Terminal-Look der App:** Verifizierung, Einladung und 2FA-Reset teilen sich jetzt ein gemeinsames, dunkles **Monospace-„Terminal-Fenster"** (Titelleiste mit Ampel-Punkten, Shell-Prompt `user@passbubble:~$ …`, eckiger grüner Button) mit den AppTheme-Farben (`#212121`/`#00E676`); der 2FA-Reset nutzt den roten Akzent. Das **echte, transparente Passbubble-Icon** (Water-Bubble + `> _`, rasterisiert aus `assets/svg/icon-extension.svg`) ist **inline per CID** eingebettet (`multipart/related`, `go:embed`) — es erscheint immer, auch wenn der Client Remote-Bilder blockiert, und ist nicht mehr das Flutter-Default-Icon
- **Share-Links in der CLI/TUI** (vorher nur Flutter): im Vault-Screen einen **Eintrag oder einen ganzen Ordner** markieren und mit `L` einen Zero-Knowledge-Link erzeugen — Ablauf wählbar (1 Tag / 7 / 30 / 90 Tage / 1 Jahr / Never). Ordner-Links bündeln alle direkt enthaltenen Einträge (wie in der Flutter-App). Das Ergebnis wird als **ASCII-QR-Code direkt im Terminal** plus voller URL angezeigt (und in die Zwischenablage kopiert). Der Link-Schlüssel lebt nur im URL-Fragment und erreicht den Server nie; identisches Format wie die Flutter-App, also vom Web-Viewer lesbar
- **QR-Code im Share-Dialog:** nach dem Erstellen eines Links wird zusätzlich ein **scanbarer QR-Code** (mit dem vollständigen Link inkl. Fragment-Key) über dem Terminal-Feld angezeigt
- **Root-Route leitet auf die Web-App um:** der nackte Host (`/`) redirectet jetzt direkt auf `/web/` (vorher 404/leer)
- **Web-App-Metadaten & Link-Vorschau:** aussagekräftige Beschreibung statt „A new Flutter project" (Manifest + `<meta description>`, Open-Graph/Twitter-Tags), Theme-Farbe an den Terminal-Look angepasst
- **Share-Viewer:** Passwort- und TOTP-Secret-Felder haben jetzt einen **Show/Hide-Button** (vorher nur maskiert ohne Aufdeckmöglichkeit)
- **Widerrufene Share-Links komplett entfernbar:** nach dem Widerruf bekommt der Eintrag in Manage → Shares einen **Remove-Button**, der den Link endgültig löscht (neuer Hard-Delete-Endpoint `DELETE /shares/links/{id}/permanent`); der Widerruf selbst bleibt ein Soft-Delete, damit der öffentliche Token sofort tot ist
- **Vault-Edit-Mode + Aktions-Sheet (Apple-Mail-Stil)** in der Flutter-App: **Long-Press** auf einen Eintrag öffnet ein Sheet mit **Share-Link / Verschieben / Duplizieren / Löschen**. Über das **Auswahl-Icon** in der AppBar startet der **Mehrfach-Auswahl-Modus** (Tippen markiert, „All" wählt alles); eine **untere Aktions-Leiste** wendet dieselben Aktionen auf alle markierten Einträge an. Verschieben re-parented die Einträge (Ziel-Ordner-Picker), Duplizieren re-verschlüsselt sie als „(copy)" im selben Ordner
- Flutter-App: **Release-Notes/Changelog werden als formatiertes Markdown gerendert** (Überschriften, Listen, **fett**, `code`, anklickbare Links) statt als Rohtext im Update-Screen
- **Account-2FA (TOTP) auf allen Plattformen** — der Login lässt sich mit einem zeitbasierten Einmalcode absichern. Zwei-Schritt-Login (Passwort → Code) über einen kurzlebigen `2fa_pending`-Token; Aktivieren/Bestätigen/Deaktivieren via Authenticator-App. **E-Mail-Reset**, falls der Authenticator verloren geht (Recovery-Link, nur nach erfolgreicher Passwortprüfung). Backend (`pquerna/otp`, Secret verschlüsselt at-rest), CLI (`account-2fa enable/disable` + Code-Schritt im Login mit Recovery-Option), Flutter (Code-Screen + Settings → Two-factor authentication) und Browser-Extension (Code-Schritt im Popup)
- **Share-Links (zero-knowledge)** — Einträge **und ganze Ordner** per Link teilen, ohne dass der Server den Inhalt sieht: der Payload wird mit einem Zufallsschlüssel verschlüsselt, der nur im URL-Fragment (`#…`) lebt. Optionales **Link-Passwort**, **maximale Aufrufzahl** und **Ablaufdatum**; Widerruf über Manage → Shares. Backend (Migration `share_links`, Handler) + Flutter (Erstellen im Eintrag-Detail bzw. in der Ordner-Ansicht, **öffentlicher Viewer** unter `/share/{token}`, Listen/Widerrufen)
- **Import/Export-Job-Ledger** — server-seitige Fortschritts-Verfolgung (verarbeitet/erstellt/aktualisiert/übersprungen/fehlgeschlagen) mit geräteübergreifender Sichtbarkeit (`POST/GET/PATCH /jobs`); der Flutter-Import legt den Job an und finalisiert ihn mit den Zählern
- Browser-Extension **Autofill-MVP**: **Zugangsdaten speichern** (erkennt einen Login auf einer Seite ohne passenden Eintrag und bietet im Popup einwilligungsbasiert an, ihn zu speichern), **manuelles Anlegen** im Popup (mit Passwort-Generator), **Detail-Ansicht** mit Passwort-Reveal/Copy und **TOTP-Code-Anzeige** (live, RFC 6238), sowie generierte **Icons** (16/48/128) — Chrome- und Firefox-Build erzeugen ein vollständiges Paket
- Settings: **konfigurierbares Auto-Lock-Intervall** in CLI und Flutter-App. CLI: im Settings-Screen mit Taste `t` durch die Presets (Off/1/5/10/15/30/60 Min) schalten, Wert wird in der Config gespeichert. Flutter-App: Settings → **Auto-lock** öffnet eine Auswahl; die App sperrt den Vault jetzt überhaupt erst bei Inaktivität (Standard 10 Min) und kehrt zum Entsperr-Screen zurück. `0`/`Off` deaktiviert das Auto-Lock
- Flutter-App: **„Lock vault"** im Settings-Screen funktioniert jetzt (löscht die privaten Schlüssel aus dem Speicher und führt zum Entsperr-Screen) statt nur eine Snackbar zu zeigen

## [2.1.0] - 2026-06-19

### Added
- CLI TUI: **Ordner-Navigation** als Drill-down wie die Flutter-App — Root zeigt Unterordner zuerst, dann Einträge; Enter/→ geht hinein, Esc/← wieder hinaus; Breadcrumb im Titel
- CLI TUI: **Sortierung** nach Name, Erstell-, Änderungsdatum und URL — per Tasten (`s` Feld, `S` Richtung, `f` Ordner-zuerst) oder Overlay-Menü (`o`); Einstellung wird in der Config gespeichert
- CLI TUI: **Ordner-CRUD** — neu (`n`), umbenennen (`e` auf Ordner), löschen (`d`, mit Schutz gegen das Löschen nicht-leerer Ordner) sowie Eintrag in Ordner **verschieben** (`m`)
- CLI TUI: **Login / Registrierung / Entsperren / Sperren / Abmelden direkt in der TUI** — kein vorheriges `pwmgr login` mehr nötig; Auth-Gate beim Start
- CLI TUI: **Auto-Lock** bei Inaktivität (Standard 10 Min) sperrt den Vault und kehrt zum Entsperr-Screen zurück
- CLI TUI: **Settings-Screen** (`.`) mit Account-/Serverinfo, Lock, Logout und **konfigurierbaren Keybindings** (umbinden, mit Esc unbinden, persistiert)
- CLI TUI: **Suche/Filter** (`/`) über alle Ordner hinweg (Name + URL)
- CLI TUI: **Quick-Copy** aus der Liste — Passwort (`y`) und Username (`u`) ohne Detailansicht; Zwischenablage wird nach 20 s automatisch geleert
- CLI TUI: **Hilfe-Overlay** (`?`) zeigt die aktuelle Tastenbelegung
- **Import: Original-Daten** (`created_at`/`updated_at`) werden übernommen statt auf den Import-Zeitpunkt gesetzt — für Psono (`create_date`/`write_date`), Bitwarden (`creationDate`/`revisionDate`), 1Password 1PUX (`createdAt`/`updatedAt`) und KeePass (Times). Backend akzeptiert die Felder beim Anlegen (`COALESCE(..., NOW())`). Greift in Flutter-App **und** CLI
- **CLI-Import: Ordner** werden jetzt berücksichtigt — Psono-Ordnerbaum, Bitwarden-Ordner (`/`-verschachtelt), KeePass-Gruppen und 1Password-Vaults werden angelegt und Einträge zugeordnet (vorher landete alles im Root)
- TOTP: Secrets werden robuster normalisiert — `otpauth://`-URLs (Periode/Stellen/Algorithmus aus der URL), mit Leerzeichen/Bindestrichen gruppierte sowie kleingeschriebene base32-Secrets werden akzeptiert

### Changed
- CLI: Die TUI lädt Einträge + Ordner jetzt direkt über den Vault (statt nur über den Keyring-Shim) und führt `created_at` / `updated_at` sowie `folder_id` bis in die Liste durch

### Fixed
- CLI Vault: `UpdateEntry` überschrieb beim Bearbeiten ungewollt die Ordnerzuordnung (Backend setzt `folder_id` immer) — der bestehende Ordner wird jetzt beibehalten; neue `MoveEntry`-Methode für gezieltes Verschieben
- CLI: `ListAllDecrypted` lieferte immer eine leere Liste (Metadaten ohne Entschlüsselung) → Import-Duplikatprüfung griff nie (Re-Importe erzeugten Dubletten) und `pwmgr export` exportierte nichts. Einträge werden jetzt korrekt entschlüsselt

## [2.0.23] - 2026-06-19

### Added
- CLI TUI: Mausklick-Support — Eintrag anklicken öffnet direkt die Detailansicht (`tea.WithMouseCellMotion`)
- CLI TUI: Zwischenablage-Kopie via `c`-Taste in der Detailansicht (wl-copy / xclip / xsel)
- CLI TUI: Listenscrolling — bei mehr Einträgen als Terminalzeilen wird gescrollt, mit Scroll-Indikator
- CLI TUI: Formularbreiten passen sich dynamisch an die Terminalbreite an (kein hartkodiertes `Width(40)` mehr)

### Fixed
- CLI TUI: Detailbox und Inhaltsboxen hatten hartkodierte Breite 50 — jetzt responsiv (`m.width - 8`)
- CLI TUI: Hilfetext war eine lange, schwer lesbare Zeile — jetzt zwei kompakte Zeilen
- CLI TUI: Toter `a`-Shortcut (verwirrende Status-Message) entfernt; `p` und `t` sind jetzt direkte Shortcuts
- CLI Crypto: `DecryptDataKey` konnte Einträge, die mit der Flutter-App erstellt wurden, nicht entschlüsseln (`encrypted key too short`). Die Flutter-App nutzt X25519-only mit rohem Shared Secret (kein HKDF, kein ML-KEM). `DecryptDataKey` erkennt jetzt automatisch das Format anhand der Länge und fällt auf den Legacy-Pfad zurück

## [2.0.17] - 2026-06-18

### Added
- Register form: live password strength bar (rot → orange → gelb → grün) mit Häkchen-Liste; erzwingt min. 12 Zeichen, min. 1 Zahl und min. 1 Sonderzeichen

### Fixed
- Release workflow: `download-artifact` filtert jetzt auf `{cli-*,flutter-*}`; das interne Docker Build Cloud Artifact (`*.dockerbuild`) hat den Release-Job nach 5 Retries zum Absturz gebracht

### Changed
- CI/Release: pub-Package-Cache zu `ci.yml` und allen Release-Build-Jobs hinzugefügt; Go-Modul-Cache-Pfad im `release.yml` Test-Job ergänzt; Linux Flutter-Build auf `awalsh128/cache-apt-pkgs-action` umgestellt

## [2.0.13] - 2026-06-18

### Added
- Email verification on registration: set `SMTP_HOST` (+ optional `SMTP_PORT`, `SMTP_USER`, `SMTP_PASSWORD`, `SMTP_FROM`, `APP_BASE_URL`) to require users to click a one-time link before their account is activated. Omitting `SMTP_HOST` preserves the previous auto-activate behaviour.
- New endpoint `GET /api/v1/auth/verify-email?token=…` — validates token, activates account, returns a browser-friendly HTML confirmation page.
- Login now returns HTTP 403 with `"email not verified — check your inbox"` when the account is still in `pending` state.
- DB migration `000002_email_verification` adds the `email_verification_tokens` table.
- [Mailpit](https://github.com/axllent/mailpit) added to the dev Docker Compose stack (`make up`) — pre-wired as SMTP backend so email verification works out of the box. Web UI at `http://localhost:8025`.

## [2.0.12] - 2026-06-18

### Fixed
- Flutter: `deriveMasterKey` tests timed out on CI (30s limit) — pass minimal Argon2id params (`memory=1024, iterations=1`) in unit tests; production hardness unchanged

## [2.0.11] - 2026-06-18

### Fixed
- CLI: `GetTimeRemaining` off-by-one — returned `period` (30) instead of 0 when `now % period == 0`, causing `TestGetTimeRemaining` to fail

### Changed
- `CLAUDE.md`: add mandatory pre-commit/pre-tag checklist (backend + CLI `-race` + flutter)

## [2.0.10] - 2026-06-18

### Added
- `deploy.sh` — fully automated server deployment script: pulls compose file, generates secrets, starts Docker stack, configures nginx vhost, obtains Let's Encrypt cert via certbot, downloads CLI binary
- `nginx/passbubble.conf` — nginx vhost template (TLS, HSTS, proxy_pass, SSE support)
- `docker-compose.server.caddy.yml` — preserved Caddy variant for fresh servers without existing reverse proxy

### Changed
- `docker-compose.server.yml`: remove Caddy service, bind backend to `127.0.0.1:8765` only (nginx handles TLS on the host)

### Fixed
- `deploy.sh`: use `maybe_sudo` helper — works as both root and sudoer
- `deploy.sh`: gracefully skip CLI download if GitHub release binary not yet available
- `deploy.sh`: read `DOMAIN`/`ADMIN_EMAIL` from existing `.env` on re-runs (no re-prompt)

## [2.0.9] - 2026-06-18

### Fixed
- CI: remove Flutter Windows build — `local_auth_windows` incompatible with MSVC 14.51 (VS 18)
- CI: fix DockerHub image name `gerry3010/passbubble-server` → `gerre01/passbubble-server` in all workflows and docs

## [2.0.8] - 2026-06-18

### Fixed
- Flutter: upgrade `local_auth` 2.3 → 3.0 to fix Windows MSVC 14.51 C++ experimental-coroutine build error in `local_auth_windows`
- Flutter: migrate `AuthenticationOptions(biometricOnly: true)` → `biometricOnly: true` (new `local_auth` 3.0 API)

## [2.0.7] - 2026-06-18

### Performance
- Docker: use `--platform=$BUILDPLATFORM` + `GOARCH=$TARGETARCH` for Go and Flutter builder stages — replaces QEMU emulation with native cross-compilation, cutting arm64 build time from ~10 min to ~1 min

## [2.0.6] - 2026-06-18

### Fixed
- CI: fix Docker build context (`context: .` + `file: backend/Dockerfile`) so `flutter_app/` is available during the image build

## [2.0.5] - 2026-06-18

### Fixed
- CLI: resolve 17 golangci-lint issues (errcheck: defer Close/Remove/ReadFull, staticcheck: De Morgan's law)
- Flutter: fix deprecated API usage in export/import tabs (`value`→`initialValue`, `activeColor`→`activeThumbColor`, `withOpacity`→`withValues`)

## [2.0.4] - 2026-06-18

### Fixed
- CI: upgrade `golangci-lint-action` v6 → v7 (v6 rejects golangci-lint v2.x)
- CI: Go version 1.24 → 1.26 to match `go.mod` (backend + CLI)
- Flutter: add missing `PbBottomNav` widget (`shared/widgets/bottom_nav.dart`)
- Flutter: add missing `job_polling_service.dart` (`runningJobsProvider`)
- Flutter: add `JobResponse`, `MySharesResponse`, `ShareLinkResponse`, `DirectShareResponse` to `models.dart`
- Flutter: add `listJobs`, `listMyShares`, `revokeShareLink`, `revokeEntryShare`, `revokeFolderShare` to `ApiClient`

## [2.0.3] - 2026-06-18

### Fixed
- CI: pin Flutter to `3.44.0` (Dart 3.10) — wildcard `3.44.x` fell back to cached `3.32.8`
- CI: restore `sdk: ^3.10.0` Dart constraint in `pubspec.yaml` (correct for Flutter 3.44.0)

## [2.0.2] - 2026-06-18

### Fixed
- CI: update GitHub Actions to Node.js 24 compatible versions (checkout v6, docker/* actions v4/v4/v4/v6/v7)
- CI: add `libsecret-1-dev` to Linux Flutter build dependencies (required by `flutter_secure_storage_linux`)
- CI: remove unused `assets:` declaration from `pubspec.yaml` (caused build warning on Windows and Docker)

## [2.0.1] - 2026-06-18

### Fixed
- CI: upgrade Flutter from 3.32.x to 3.44.x to satisfy the `sdk: ^3.10.4` Dart constraint declared in `pubspec.yaml`

### Added
- `docker-compose.server.yml` — standalone production compose file using the DockerHub image (no source or Go needed on the server)
- `docs/server-deployment.md` — full server setup, update, and pitfall guide

## [2.0.0] - 2026-06-16

### Changed
- **Breaking: full architecture rewrite to a self-hosted client/server monorepo.** The single-binary, system-keyring-backed CLI is replaced by:
  - `backend/` — Go REST API server (PostgreSQL + Redis), with end-to-end encryption (X25519 + ML-KEM-768 hybrid KEM, AES-256-GCM, Argon2id) so the server never sees plaintext
  - `cli/` — Go CLI/TUI (`pwmgr`) acting as an API client, with all crypto performed client-side
  - `flutter_app/` — Flutter app serving the web UI (`/web/*`) and admin panel (`/admin/*`), embedded into the backend binary at build time
- Added multi-user support: invitations, sharing, folders, admin roles
- Added Docker Compose deployment (`./setup.sh`) with first-run bootstrap admin registration
- Removed the old GNOME-Keyring/system-keychain storage backend, the standalone single-binary CLI source (`cmd/`, `internal/`, `pkg/`), and associated build artifacts (`build/`, `dist/`) — superseded by the new monorepo layout

## [1.0.0] - 2025-10-09

### Added
- Initial release of the Password Manager Go Edition
- Complete rewrite from Bash to Go with modern architecture
- Interactive Bubble Tea TUI with beautiful interface
- Secure password storage using system keyring (GNOME Keyring, Keychain, Credential Manager)
- TOTP 2FA support with live code generation and visual countdown
- Advanced password generation with multiple types (strong, memorable, passphrase)
- Encrypted backup and restore functionality with GPG support
- Cross-platform compatibility (Linux, macOS, Windows)
- Comprehensive CLI interface with all password management operations
- Search and organization capabilities
- Real-time TOTP code refresh with visual progress indicators

### Fixed
- **Critical**: TOTP progress bar auto-refresh functionality
  - Fixed timer continuation issue where progress bar would stop updating
  - Progress bar now properly counts down from 30 to 0 automatically
  - TOTP codes refresh seamlessly when countdown expires
  - Implemented proper Bubble Tea command batching for smooth UI updates

### Technical Details
- Built with Go 1.21+ and modern dependencies
- Uses Cobra for CLI framework and Bubble Tea for TUI
- Secure keyring integration via zalando/go-keyring
- TOTP implementation using pquerna/otp library
- Comprehensive test coverage for all core functionality
- Cross-platform build system with release automation

### Migration from Bash Version
- All Bash functionality preserved and enhanced
- Improved performance and reliability
- Better error handling and user experience
- Backward compatible with existing password entries
- Enhanced security with proper secret handling

### Documentation
- Comprehensive README with installation and usage instructions
- Inline help and examples for all commands
- Architecture documentation and development guide
- Security best practices and recommendations

### Dependencies
- github.com/charmbracelet/bubbletea (TUI framework)
- github.com/charmbracelet/lipgloss (TUI styling)
- github.com/spf13/cobra (CLI framework)
- github.com/zalando/go-keyring (secure storage)
- github.com/pquerna/otp (TOTP implementation)
- github.com/spf13/viper (configuration management)