// Post-build script: copy manifest + icons + wasm to dist/
// Usage: node scripts/build.js [chrome|firefox]

import { copyFileSync, mkdirSync, existsSync, renameSync, rmSync } from 'fs';
import { join, dirname } from 'path';
import { fileURLToPath } from 'url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const root = join(__dirname, '..');

const browser = process.argv[2] ?? 'chrome';
const outDir = join(root, 'dist', browser);

function copy(src, dest) {
  const destDir = dirname(dest);
  if (!existsSync(destDir)) mkdirSync(destDir, { recursive: true });
  copyFileSync(src, dest);
  console.log(`  copied ${src.replace(root, '')} → ${dest.replace(root, '')}`);
}

// Manifest
copy(join(root, `manifest.${browser}.json`), join(outDir, 'manifest.json'));

// Icons (create placeholder if not present)
const iconsDir = join(root, 'public', 'icons');
const outIconsDir = join(outDir, 'icons');
mkdirSync(outIconsDir, { recursive: true });
for (const size of [16, 48, 128]) {
  const src = join(iconsDir, `icon${size}.png`);
  if (existsSync(src)) {
    copy(src, join(outIconsDir, `icon${size}.png`));
  } else {
    console.warn(`  ⚠ Missing icon${size}.png — add to public/icons/`);
  }
}

// WASM
const wasmDir = join(root, 'public', 'wasm');
const outWasmDir = join(outDir, 'wasm');
mkdirSync(outWasmDir, { recursive: true });
if (existsSync(wasmDir)) {
  const { readdirSync } = await import('fs');
  for (const f of readdirSync(wasmDir)) {
    if (f.endsWith('.wasm')) {
      copy(join(wasmDir, f), join(outWasmDir, f));
    }
  }
}

// HTML entry points: Vite emits them under dist/<browser>/src/<name>/index.html
// (mirroring the rollup input paths), but the manifest references them at
// <name>/index.html. Move them up and drop the leftover src/ dir.
for (const name of ['popup', 'options', 'fill-iframe']) {
  const from = join(outDir, 'src', name, 'index.html');
  const to = join(outDir, name, 'index.html');
  if (existsSync(from)) {
    if (!existsSync(dirname(to))) mkdirSync(dirname(to), { recursive: true });
    renameSync(from, to);
    console.log(`  moved /src/${name}/index.html → /${name}/index.html`);
  }
}
const srcDir = join(outDir, 'src');
if (existsSync(srcDir)) rmSync(srcDir, { recursive: true, force: true });

console.log(`\n✓ ${browser} build ready at dist/${browser}/`);
