# Passbubble app icon — source & Liquid Glass layers

Single source of truth for the app icon is [`flutter_app/assets/svg/icon.svg`](../../flutter_app/assets/svg/icon.svg)
(dark base, neon padlock in a water bubble, circuit traces). The rendered
`AppIcon.appiconset` PNGs for **iOS** and **macOS** are generated from it.

`preview.png` shows the assembled 1024×1024 icon.

## Regenerate the flat AppIcon sets

```bash
# macOS (16…1024)
SVG=flutter_app/assets/svg/icon.svg
OUT=flutter_app/macos/Runner/Assets.xcassets/AppIcon.appiconset
for s in 16 32 64 128 256 512 1024; do rsvg-convert -w $s -h $s "$SVG" -o "$OUT/app_icon_$s.png"; done
```

(iOS uses the standard `Icon-App-*` sizes in
`flutter_app/ios/Runner/Assets.xcassets/AppIcon.appiconset`.)

## Liquid Glass (`layers/`) — for Icon Composer (Xcode 26)

iOS 26 / macOS 26 icons are authored in **Icon Composer** as stacked layers so
the system can apply the glass / specular / dark / tinted variants. The design
is split into four layers (all 1024×1024, viewBox 256), bottom → top:

| # | file | alpha | role |
|---|------|-------|------|
| 1 | `1-background` | opaque | dark base + scanlines (gets the rounded-rect mask) |
| 2 | `2-bubble`     | transparent | bubble membrane: translucent fill + neon stroke + specular arc |
| 3 | `3-circuits`   | transparent | four circuit traces + chip pads |
| 4 | `4-padlock`    | transparent | the padlock glyph — the natural "hero" layer for the glass treatment |

Each layer ships as **`.svg`** (edit source) and a rendered **`.png`** (drop
straight into Icon Composer). Re-render after editing:

```bash
cd design/icon/layers
for f in 1-background 2-bubble 3-circuits 4-padlock; do rsvg-convert -w 1024 -h 1024 "$f.svg" -o "$f.png"; done
```

### Assembling in Icon Composer
1. New icon → drag the four PNGs in as separate layers in the order above.
2. Set **`1-background`** as the Background; layers 2–4 as Foreground groups.
3. Apply Liquid Glass; put the depth/specular emphasis on **`4-padlock`**.
4. Two caveats baked into the notes in each SVG:
   - The **glow** (neon blur) is intrinsic to the brand look — disable it per
     layer if you'd rather the system's own specular dominate.
   - The **circuit traces touch the icon edge**. Liquid Glass clips to the
     squircle, so pull them inward or hide `3-circuits` if the mask crops them
     awkwardly.
5. Export the `.icon` and add it to both the iOS and macOS Runner targets.

## Assembled icon — `design/icon-composed.icon` (source of truth for the app)

The finished Icon Composer document lives at `design/icon-composed.icon`
(padlock group inset to 88 % so the Liquid Glass parallax never clips the
shackle). It is **deployed** as `AppIcon.icon` into both Runner targets:

- `flutter_app/ios/Runner/AppIcon.icon`
- `flutter_app/macos/Runner/AppIcon.icon`

Each Runner target references its copy as a resource and sets
`ASSETCATALOG_COMPILER_APPICON_NAME = AppIcon`; the old `AppIcon.appiconset`
was removed (Icon Composer bakes the legacy/flattened renderings into the
`.icon`, which `actool` expands for pre-26 OSes). **Note:** a `.icon` must sit
at the target root, *not* inside `Assets.xcassets` — `actool` silently ignores
a nested `.icon` and the app ends up with no icon.

After editing `icon-composed.icon` in Icon Composer, re-deploy:

```bash
for d in ios macos; do
  rm -rf "flutter_app/$d/Runner/AppIcon.icon"
  cp -R design/icon-composed.icon "flutter_app/$d/Runner/AppIcon.icon"
done
```
