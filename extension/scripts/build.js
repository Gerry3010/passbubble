// Post-build script: copy manifest + icons + wasm to dist/
// Usage: node scripts/build.js [chrome|firefox]

import { copyFileSync, mkdirSync, existsSync } from 'fs';
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

console.log(`\n✓ ${browser} build ready at dist/${browser}/`);
