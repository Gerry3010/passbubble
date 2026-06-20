# Icons & Brand Assets

All raster icons are **generated from SVG** at build time. The SVGs in
`assets/svg/` are the single source of truth — the rasterized PNGs are build
artifacts and are **never committed** (they are `.gitignore`d). This avoids
drifting copies: change the SVG once and every consumer re-renders.

## Sources of truth

| File | Purpose | Background |
|------|---------|------------|
| `assets/svg/icon.svg` | Flutter **app** icon (mobile/desktop/web favicon) | opaque |
| `assets/svg/icon-extension.svg` | Browser **extension** + transactional **email** icon | transparent |
| `assets/svg/banner.svg` | README / marketing banner | — |

> Two SSOTs by design: the app icon has a background; the extension/email icon
> is transparent and scaled slightly larger.

## Distribution map

```
assets/svg/icon.svg                    (opaque — app)
  ├─ make sync-assets ──► flutter_app/assets/svg/icon.svg ...... [gitignored]
  │                    ─► flutter_app/web/favicon.svg .......... [gitignored]
  │
  └─ make launcher-icons ► flutter_app/android/.../mipmap-mdpi/ic_launcher.png    [gitignored]
      (rsvg 48/72/96/      flutter_app/android/.../mipmap-hdpi/ic_launcher.png    [gitignored]
       144/192)           flutter_app/android/.../mipmap-xhdpi/ic_launcher.png   [gitignored]
                          flutter_app/android/.../mipmap-xxhdpi/ic_launcher.png  [gitignored]
                          flutter_app/android/.../mipmap-xxxhdpi/ic_launcher.png [gitignored]

assets/svg/icon-extension.svg          (transparent — extension + mail)
  ├─ make sync-assets ─► extension/icons/icon.svg ............... [gitignored]
  │
  ├─ make icons ──────► extension/public/icons/icon16.png ...... [gitignored]
  │   (rsvg 16/48/128)  extension/public/icons/icon48.png ...... [gitignored]
  │                     extension/public/icons/icon128.png ..... [gitignored]
  │                       └─► manifest.json (action + icons)
  │                       └─► dist/chrome|firefox/icons/  (extension build)
  │
  └─ make mailer-icon ► backend/internal/mailer/passbubble-icon.png  [gitignored]
      (rsvg 192×192)        └─► //go:embed → server binary
                                  └─► emails, inline via cid:passbubble-icon
```

## Make targets

| Target | Generates | Requires |
|--------|-----------|----------|
| `make sync-assets` | copies SVGs into sub-projects | — |
| `make mailer-icon` | the email PNG (`//go:embed` asset) | `rsvg-convert` |
| `make launcher-icons` | the 5 Android launcher mipmaps | `rsvg-convert` |
| `make icons` | all extension PNGs **+** `mailer-icon` **+** `launcher-icons` | `rsvg-convert` |

> **Android builds:** the launcher mipmaps are gitignored generated artifacts,
> and no CI job builds an Android target — so nothing regenerates them
> automatically. Run `make launcher-icons` (or `make icons`) before a local
> `flutter build apk`/`appbundle`, or the build fails on missing `ic_launcher`
> resources.

`rsvg-convert` comes from the `librsvg` toolset:
- Arch/Manjaro: `pacman -S librsvg`
- Debian/Ubuntu: `apt-get install librsvg2-bin`
- Alpine: `apk add rsvg-convert`

## Why the email icon needs generating before every backend build

`backend/internal/mailer/mailer.go` embeds the PNG at compile time:

```go
//go:embed passbubble-icon.png
var passbubbleIcon []byte
```

Because the PNG is **not** in git, it must exist whenever the mailer package
compiles, or the build fails with `pattern passbubble-icon.png: no matching
files found`. Every backend-compiling path therefore generates it first:

| Where | How |
|-------|-----|
| Local `make dev` / `build-backend` / `test-backend` | depend on `mailer-icon` |
| Local `make build-extension` | depends on `icons` |
| Manual `cd backend && go build` | run `make mailer-icon` first (see CLAUDE.md checklist) |
| CI `ci.yml` (Test backend) | `apt-get install librsvg2-bin` + `make mailer-icon` |
| Release `release.yml` (test job) | same as CI |
| Docker image (`backend/Dockerfile`) | `apk add rsvg-convert`, render before `go build` (build context is the repo root, so `assets/` is reachable) |

The CLI does **not** import the mailer package, so CLI builds/tests need no icon.

## Changing the icon

1. Edit `assets/svg/icon.svg` and/or `assets/svg/icon-extension.svg`.
2. Run `make icons` to regenerate everything locally.
3. Rebuild the extension (`make build-extension`) and reload it in the browser.
4. Commit only the SVG — the PNGs stay untracked and CI/Docker regenerate them.
