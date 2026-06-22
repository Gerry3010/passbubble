# Backup-Strategie für Passbubble (gesamter Server, alle User)

> Status: **geplant** — Umsetzung startet, sobald IONOS S3 Object Storage eingerichtet ist.

## Kontext

Auf `passbubble.geraldhofbauer.net` (IONOS-Server, `/opt/passbubble`, docker compose)
liegen bereits echte Produktivdaten. Aktuell gibt es **kein serverweites Backup** — nur
ein app-level CLI-Export (`pwmgr backup`) pro User, das keine Disaster-Recovery für alle
User leistet. Ziel: eine automatische, verschlüsselte, off-site gelagerte Backup-Lösung,
mit der sich auf einem frischen Server **jeder User wieder anmelden und alle Daten
entschlüsseln** kann — auch wenn der Server komplett verloren geht.

### Was gesichert werden muss (Ergebnis der Code-Analyse)

- **PostgreSQL `pwmgr` ist die einzige Quelle der Wahrheit.** Ein vollständiger
  `pg_dump` enthält *alles*: `users` (inkl. master-key-gewrappte Private Keys + KDF-
  Parameter), `entries` (E2E-verschlüsselt), `entry_keys` (per-User gewrappte Data-Keys),
  `folders`, `*_permissions`, `share_links`, TOTP-Secrets. Redis ist ephemer/ungenutzt →
  **kein Backup nötig**. Kein Datei-Storage außerhalb der DB (`/data/backups`-Volume ist
  nur das separate CLI-Export-Feature).
- **`JWT_SECRET` ist mit-kritisch:** verschlüsselt die TOTP-2FA-Secrets *at rest*
  (`SHA256("passbubble-totp-secret-v1:"+JWT_SECRET)`, `backend/internal/api/handlers/totp.go:95`).
  Verlust ⇒ nur 2FA-Secrets unbrauchbar (User können per Recovery-Mail 2FA zurücksetzen);
  alles andere bleibt über das Master-Passwort wiederherstellbar. ⇒ **`.env` mit
  `JWT_SECRET` + `POSTGRES_PASSWORD` muss mitgesichert werden**, sonst startet ein
  restaurierter Server nicht und TOTP bleibt undecodierbar.
- **Metadaten liegen im Klartext** in der DB (`entries.name`, `entries.url`,
  `folders.name`, `match_patterns`). Ein Dump verrät *wo* du Accounts hast ⇒ Backups
  **müssen verschlüsselt at-rest + off-site** liegen.

### Gewählte Strategie

- **Tooling:** `age` (asymmetrisch) + `rclone`. Der Server hat **nur den age-Public-Key**
  → kann eigene Backups nie entschlüsseln (Zero-Trust bei Server-Kompromittierung).
  Restore = ein `age -d` Befehl, langzeit-robust, kein Format-Lock-in.
- **Ziel (3-2-1):** IONOS S3 Object Storage (off-site, EU, S3-kompatibel → rclone) **und**
  zwei automatische lokale Kopien auf den **zwei Synology-NAS** (NAS ziehen den
  S3-Bucket via Synology **Cloud Sync** — kein eigenes Script nötig).
- **Frequenz/Retention:** täglich, GFS = 7 täglich + 4 wöchentlich + 6 monatlich.

---

## Architektur

```
IONOS-Server (/opt/passbubble)              IONOS S3 (off-site)        2× Synology NAS (lokal)
┌──────────────────────────────┐            ┌──────────────┐          ┌──────────────────────┐
│ cron 03:30 → pb-backup.sh     │            │  bucket:     │ Cloud    │ NAS-1: Cloud Sync ◀── │
│  pg_dump(-Fc) ┐               │  rclone    │  pb-backups/ │ Sync     │   S3 → Shared Folder  │
│  + .env-Snap  ├─tar─►age -R──►│ ──push────▶│  *.tar.age   │ ──pull──▶│ NAS-2: 2. Kopie       │
│  + manifest   ┘  (nur PubKey) │            │ (versioniert)│          │   (Cloud Sync o.      │
│  GFS-Prune lokal              │            └──────────────┘          │    Replication v. NAS-1)│
│  → healthchecks.io ping       │                                      │  age-IDENTITY ✦ offline │
└──────────────────────────────┘                                      └──────────────────────┘
✦ age-Private-Key liegt NUR offline/im Passwort-Manager, NIE auf dem Server (und nicht
  zwingend auf den NAS — die gespeicherten *.tar.age sind ohnehin age-verschlüsselt).
```

**Backup-Artefakt:** ein einziges `pb-backup-<UTC>.tar.age` pro Lauf, das
`db.dump` (pg_dump custom format) + `env.snapshot` (Server-`.env`) + `manifest.txt`
(Server-Version, Zeitstempel) bündelt → mit `age` an den Recipient-PubKey verschlüsselt.
Single-Artifact-Restore.

---

## Umzusetzende Dateien (alle committed, außer Secrets)

### Scripts — `scripts/backup/` (neu)

- **`pb-backup.sh`** — läuft auf dem IONOS-Server (cron). `set -euo pipefail`, sourct
  `backup.env`. Schritte:
  1. `docker compose exec -T postgres pg_dump -U pwmgr -Fc pwmgr` → temp `db.dump`
     (pg_dump 16 *aus dem Container* → garantierter Versions-Match, kein Host-pg nötig).
  2. Snapshot von `/opt/passbubble/.env` + `manifest.txt` schreiben.
  3. `tar` der drei Dateien → `age -R $BACKUP_AGE_RECIPIENTS` → `pb-backup-<UTC>.tar.age`
     in `$BACKUP_LOCAL_DIR` (z.B. `/opt/passbubble/db-backups/`, eigener Host-Ordner —
     **nicht** das bestehende `backups`-Volume, um Verwechslung zu vermeiden).
  4. `rclone copyto` → `$BACKUP_S3_REMOTE` (IONOS S3, HTTPS).
  5. **GFS-Prune** lokal (7 täglich/4 wöchentlich/6 monatlich behalten; Wochen-/
     Monats-Marker über Dateiname-Datum). Remote-Retention via S3-Lifecycle (s.u.).
  6. Erfolg → `curl $BACKUP_HEALTHCHECK_URL` (Dead-Man's-Switch). Fehler → non-zero
     exit (cron-Mail) + `curl $BACKUP_HEALTHCHECK_URL/fail`. Log nach `/var/log/pb-backup.log`.

- **`pb-restore.sh`** — Restore aus einem Artefakt. `age -d -i <identity>` → untar →
  `pg_restore --clean --if-exists -U pwmgr -d pwmgr db.dump` + `.env` zurückspielen.
  Flag **`--verify <artifact>`**: restauriert in einen **Wegwerf-`postgres:16-alpine`-
  Container**, prüft Row-Counts (`users`, `entries`, `entry_keys` > 0), reportet, räumt
  auf — non-destruktiv. „Ein nie wiederhergestelltes Backup ist kein Backup."

- **Lokale Kopie auf die Synology-NAS — primär via Synology Cloud Sync (kein Script):**
  Auf NAS-1 das Paket **Cloud Sync** installieren, Verbindungstyp „S3 Storage" (Custom
  Endpoint = IONOS S3), **Download-only / remote→lokal**, Zielordner z.B.
  `/volume1/passbubble-backups`. Das ersetzt einen eigenen Puller komplett (GUI,
  geplant, robust). NAS-2 erhält die zweite Kopie entweder per eigenem Cloud-Sync-Job
  oder per **Snapshot Replication / Shared-Folder-Sync von NAS-1**.
- **`pb-pull.sh`** (committed, optionaler Fallback) — für eine NAS *ohne* Cloud Sync
  (DSM Task Scheduler + rclone via Community-Paket/Docker) oder einen normalen Rechner:
  `rclone copy` IONOS S3 → lokaler Ordner + GFS-Prune lokal. Kann die age-Identity für
  periodischen `--verify` halten.

- **`README.md`** — Runbook (s. „Betrieb & Schlüssel-Custody").

### Konfig & Secrets (Muster wie `deploy.local.mk`)

- **`scripts/backup/backup.env.example`** (committed) — dokumentiert die Variablen:
  `BACKUP_AGE_RECIPIENTS` (Pfad zur recipients-Datei, *Public Key* — nicht geheim),
  `BACKUP_S3_REMOTE` (z.B. `ionos:pb-backups`), `BACKUP_LOCAL_DIR`,
  `BACKUP_HEALTHCHECK_URL`, `BACKUP_KEEP_DAILY/WEEKLY/MONTHLY`, `COMPOSE_DIR=/opt/passbubble`.
- **`backup.env`** (real, gitignored) — liegt auf Server bzw. lokal.
- **`.gitignore`-Ergänzung:** `backup.env`, `*.age`, `*-identity.txt`, `rclone.conf`.
- **`.env.example`-Ergänzung:** kurzer Kommentarblock, der auf `scripts/backup/` verweist.

### Make-Targets (Makefile, dünne Wrapper im Stil der `deploy-*`-Targets)

- `make backup` → `pb-backup.sh` (manuell/ad-hoc, auch über SSH auf dem Server).
- `make backup-verify FILE=...` → `pb-restore.sh --verify`.
- `make restore FILE=...` → `pb-restore.sh` (mit Bestätigungs-Prompt, da destruktiv).
- Eintrag in der `make help`-Ausgabe + `.PHONY`.

---

## Betrieb & Schlüssel-Custody (im README dokumentiert)

1. **age-Keypair erzeugen:** `age-keygen -o pb-backup-identity.txt` (gibt Public Key aus).
   - **Public Key (recipient)** → Server `backup.env`. Verschlüsselt die Backups.
   - **Private Identity** → **nur lokal/offline**, in ≥2 sicheren Orten (eigener
     Passwort-Manager + verschlüsselter USB/Ausdruck im Safe). **Niemals auf den IONOS-Server.**
2. **Server-`.env` einmalig offline sichern** (JWT_SECRET, POSTGRES_PASSWORD) — sie wandert
   zwar in jedes Backup, aber eine separate Offline-Kopie schützt vor dem Henne-Ei-Problem.
3. **IONOS S3 einrichten:** Bucket `pb-backups` anlegen, S3-Access-Key erzeugen, auf dem
   Server `rclone config` (Provider „Other/IONOS", S3-Endpoint + Keys). **Härtung:**
   Bucket-**Versionierung + Lifecycle-Expiration** aktivieren (z.B. nicht-aktuelle
   Versionen nach 90 Tagen löschen) ⇒ ein kompromittierter Server kann Backups nicht
   *endgültig* löschen. Wenn IONOS es erlaubt: Server-Key auf write/put **ohne delete**
   scopen; Remote-Pruning dann vom vertrauenswürdigen lokalen Puller.
4. **Zeitplanung einrichten:**
   - Server: cron `30 3 * * *` → `pb-backup.sh` (root oder deploy-User).
   - NAS-1: Synology **Cloud Sync**, Zeitplan z.B. täglich ab 05:30 (nach dem Server-Push),
     Download-only von IONOS S3. NAS-2: eigener Cloud-Sync-Job oder Replication von NAS-1.
   - **Zwei NAS optimal nutzen:** Wenn möglich **NAS-2 an einem anderen Ort** (Büro/
     Verwandte) für Geo-Redundanz — zwei NAS im selben Raum schützen vor Serverausfall,
     aber nicht vor Brand/Diebstahl am Standort. IONOS S3 deckt den Off-site-Fall ohnehin ab;
     ein zweiter Standort fürs NAS ist Bonus.
5. **Monitoring:** healthchecks.io (kostenlos) als Dead-Man's-Switch — Alarm-Mail, wenn ein
   täglicher Ping ausbleibt (fängt *stilles* Backup-Versagen ab, die häufigste Backup-Falle).
6. **Restore-Drill:** quartalsweise echtes DR-Rehearsal (s. Verifikation Schritt 3).

---

## Verifikation (End-to-End)

1. **Dry-Run lokal:** dev-Stack `make up` starten, `pb-backup.sh` gegen das lokale compose
   laufen lassen → `pb-backup-<UTC>.tar.age` entsteht. Mit der Identity `age -d` + `tar -t`
   prüfen, dass `db.dump` + `env.snapshot` + `manifest.txt` enthalten sind.
2. **Verify-Restore:** `make backup-verify FILE=<artifact>` → Wegwerf-Postgres-Container,
   Row-Counts für `users`/`entries`/`entry_keys` > 0 und plausibel.
3. **Voller DR-Test:** frischen compose-Stack in Scratch-Verzeichnis hochziehen → `pb-restore.sh`
   → backend startet, Migrationen laufen (idempotent) → mit einem **Test-User anmelden** und
   prüfen, dass Einträge **entschlüsselt** werden. Das beweist die ganze Kette
   Master-Passwort → KDF → Private-Key → `entry_keys` → Eintrag, plus `JWT_SECRET`-Pfad.
4. **Off-site bestätigen:** S3-Objekt vorhanden (`rclone ls $BACKUP_S3_REMOTE`), die
   `*.tar.age` per Cloud Sync auf **beiden NAS** angekommen, healthchecks.io-Ping „up".

---

## Bewusst NICHT im Scope

- Redis-Backup (ephemer/ungenutzt).
- VM-Snapshots auf IONOS-Infra-Ebene — nette Ergänzung, aber nicht anwendungskonsistent
  und kein portables, verschlüsseltes Backup; ersetzt diese Lösung nicht.
- Sessions/Invitations/Email-Tokens/Job-History — werden vom `pg_dump` zwar miterfasst,
  sind aber für ein Restore unkritisch (kurzlebig).
