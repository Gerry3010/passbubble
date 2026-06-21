# Passbubble Design Guidelines

The **browser extension is the design source of truth.** Every other surface — the
Flutter app, the embedded `/web` build, the admin panel, and the CLI/TUI — aligns
to the look and language defined here. The canonical tokens live in
[`extension/src/shared/theme.ts`](../extension/src/shared/theme.ts); this document
explains them and the rules for applying them elsewhere.

The aesthetic is a **phosphor terminal**: bright green on near-black, monospace
everywhere, compact, minimal rounding, and shell-prompt language (`passbubble:~$`,
`./vault`, `# account`, `grep entries…`) with text glyphs instead of emoji.

---

## 1. Color tokens

| Token | Hex | Role |
|---|---|---|
| `bg` | `#0a0a0a` | page background |
| `surface` | `#0e140f` | cards, sections, input fill |
| `green` | `#00ff41` | primary accent — headers, focus, interaction, key text |
| `greenDim` | `#19a23a` | subtler green (hover, secondary accent) |
| `muted` | `#5f8c6a` | secondary / label text |
| `border` | `#1d3a24` | borders, dividers |
| `borderBright` | `#00ff41` (web: `#00ff4155`) | focus glow |
| `red` | `#ff5f56` | error / destructive |
| `amber` | `#ffb000` | warning, password-strength mid |

### Body-text rule: extension-faithful vs. full-screen

- **Extension & CLI** (small/dense surfaces): **all text is green** (`#00ff41`),
  faithful to the terminal look.
- **Full-screen surfaces** (Flutter app, `/web`, admin panel): keep **light-gray
  body text** (`#E0E0E0`) and use `green` only for accents, headers, focus, and
  interactive elements. A whole phone/desktop screen of pure green is fatiguing —
  green is the accent, not the body. Secondary text uses `muted` (`#5f8c6a`).

Dark mode only. There is no light theme.

---

## 2. Typography

**JetBrains Mono is the canonical brand font** on every surface. (The extension
ships `Courier New` as a web-safe fallback only; treat JetBrains Mono as the
intended face.)

| Level | Size | Weight |
|---|---|---|
| Page title | 24px | 700 |
| Section heading | 18px | 700 |
| Subsection | 15–16px | 700 |
| Body / inputs / buttons | 13px | 400 |
| Small / labels | 12px | 400 |
| Tiny / meta | 11px | 400 |

### The `0`/`O` rule

**Never use the generic `fontFamily: 'monospace'`** (or any non-disambiguating
mono) for content where a user reads or copies exact characters — generated
passwords, tokens, recovery codes, share links. The platform default monospace
often renders `0` and `O` (and `1`/`l`/`I`) identically. Always use **JetBrains
Mono**, whose zero is clearly slashed/dotted. In Flutter use
`GoogleFonts.jetBrainsMono(...)`, never the literal `'monospace'`.

---

## 3. Shell-prompt language

This is the most distinctive thing the extension does, and the main point of
alignment. Apply it on **all** surfaces (Flutter & admin included).

- **Screen headers:** `passbubble:~$ <screen>` — the `passbubble:~$` prefix in
  `muted`, the action word in `green`. Examples: `passbubble:~$ vault`,
  `passbubble:~$ login`, `passbubble:~$ settings`, `passbubble:~$ admin`.
- **Section headings:** `# <name>` — e.g. `# account`, `# security`,
  `# pin quick-unlock`, `# keybindings`.
- **Tabs / nav as paths:** `./vault`, `./generate`, `./manage`, `./users`,
  `./invitations`.
- **Search placeholder:** `grep entries…`.
- **Glyphs, not emoji:**
  - `›` list cursor / forward chevron, `‹` back
  - `✓` success, `✗` failure/error
  - `$` / `#` / `>_` shell prompts
  - status line prefix: `› `
- **Entry-type tags** (replace 🔑/🔐/🗝️/📝): `[pw]`, `[2fa]`, `[key]`, `[txt]`,
  `[?]` unknown. Folder/backup markers: `[dir]`, `[~]` (root), `[bak]`.

Keep functional icons (copy/edit/back buttons, etc.) where they're real
affordances — it's emoji **labels and decorative icons** that get replaced with
glyphs/tags.

---

## 4. Components

| Component | Style |
|---|---|
| **Primary button** | `green` fill, `bg`-colored text, 700 weight, 2px radius; hover brightens |
| **Ghost button** | transparent fill, `border` outline, `green` text, 2px radius |
| **Input** | `surface` fill, `border` outline; on focus → `green` border + glow |
| **Card / section** | `surface` fill, 1px `border`, 4px radius, flat (no shadow) |
| **Tab** | active = `green` text + 2px `green` underline; inactive = `muted` |
| **Badge / chip** | small, 1px `border`, recolored per semantic token |

Spacing is on an 8px base unit; layouts are compact. Radius: **2px** for
inputs/buttons, **4px** for cards. No drop shadows (focus glow only).

---

## 5. How this maps per surface

- **Flutter app + `/web`** — tokens live in
  [`flutter_app/lib/core/theme/app_theme.dart`](../flutter_app/lib/core/theme/app_theme.dart)
  as `AppTheme.*` constants; the `ThemeData` ripples them across Material widgets.
  Base text uses `GoogleFonts.jetBrainsMonoTextTheme()`. Body text stays `onBg`
  (`#E0E0E0`); `green` is the accent (decision: accent-green + light text).
- **Admin panel** — CSS custom properties in `:root` of
  [`backend/internal/static/admin/index.html`](../backend/internal/static/admin/index.html)
  (`--bg`, `--surface`, `--primary`, …). Single static file, no build step.
- **CLI/TUI** — tokens and shared Lip Gloss styles live in
  `cli/internal/tui/theme.go`. Lip Gloss accepts truecolor hex
  (`lipgloss.Color("#00ff41")`) and auto-degrades to ANSI on limited terminals.
  Plain Cobra command output (`cli/internal/cli/`) stays **uncolored** so it
  remains scriptable/pipeable.
- **Extension** — the reference. Not changed by alignment work.

---

## 6. License header

Every `.go`, `.ts`, and `.tsx` file must start with the AGPL v3 header (see
[`CLAUDE.md`](../CLAUDE.md)). Markdown docs like this one do not need it.
