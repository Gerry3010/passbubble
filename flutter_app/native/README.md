# Native hybrid-KEM library (`passbubble_crypto`)

A Go [`c-shared`](https://pkg.go.dev/cmd/go#hdr-Build_modes) library that exposes
the Passbubble hybrid KEM (X25519 + ML-KEM-768) to the Flutter app via
`dart:ffi` on native platforms. It is a **thin wrapper around
`backend/pkg/crypto`** — the single source of truth — so the wire format is
identical to the CLI, backend and browser extension.

Flutter **web** cannot use FFI; it uses the JS implementation instead
(`lib/core/crypto/ml_kem_web.dart`).

## Exposed functions

See `passbubble_crypto/libpassbubble_crypto.h` (generated):

- `pb_generate_mlkem768` — fresh ML-KEM-768 keypair
- `pb_encrypt_data_key` — hybrid wrap (X25519-only fallback when no ML-KEM key)
- `pb_decrypt_data_key` — unwrap, auto-detecting hybrid vs. legacy X25519-only
- `pb_free` — free returned buffers

Dart bindings: `lib/core/crypto/ml_kem_ffi.dart`.
Round-trip + Flutter-legacy-interop test: `test/ml_kem_ffi_test.dart`.

## Building

The compiled libs are **gitignored build artifacts** (like the brand icons) —
regenerate them via the repo-root Makefile (or `./build.sh` directly):

```bash
make native-crypto          # host desktop (.so/.dylib/.dll) → passbubble_crypto/build/   [gitignored]
make native-crypto-android  # all Android ABIs → android/app/src/main/jniLibs/<abi>/      [gitignored, needs ANDROID_NDK_HOME]

# equivalently, the underlying script:
./build.sh                  # host
./build.sh android          # Android ABIs
```

> Run `make native-crypto` before `flutter test` (the FFI test skips gracefully
> when the host lib is absent) or a desktop run, and `make native-crypto-android`
> before a local APK/AAB build. CI builds no Android/desktop target, so nothing
> regenerates these automatically. The **web** bundle
> (`flutter_app/web/passbubble_crypto.js`, built with `make web-crypto`) stays
> committed because `flutter build web` consumes it in environments without the
> node/shared-ts toolchain.

### Per-platform bundling

| Platform | Artifact | How it's loaded / bundled |
|----------|----------|---------------------------|
| Android  | `lib*.so` per ABI in `android/app/src/main/jniLibs/` | packaged automatically; `DynamicLibrary.open('libpassbubble_crypto.so')` |
| Linux    | `lib*.so` | copy into the bundle via `linux/CMakeLists.txt` `install(...)`; `DynamicLibrary.open('libpassbubble_crypto.so')` |
| Windows  | `*.dll`  | copy next to the runner via `windows/CMakeLists.txt`; `DynamicLibrary.open('passbubble_crypto.dll')` |
| macOS    | static `.a` / xcframework | link in the Runner target; `DynamicLibrary.process()` |
| iOS      | static `.a` (`go build -buildmode=c-archive`, per-arch, `lipo`/xcframework) | link in the Runner target; `DynamicLibrary.process()` |

Locally, `make native-crypto` covers the host so `flutter test` and desktop runs
work; tests point at the built `.so` via `mlKemLibraryPathOverride` /
`PASSBUBBLE_CRYPTO_LIB`, and the FFI test **skips** when it's absent.

## Release-pipeline integration (status + TODO)

**Today the CI/release pipeline does _not_ build any of these libs.** It stays
green anyway because:

- the Docker **server** image embeds the Flutter **web** app, which only needs
  the **committed** `web/passbubble_crypto.js` bundle (no native lib); and
- the release builds **no Android target at all**, and the desktop (Linux/macOS)
  release jobs don't yet bundle the native `.so`/`.dylib`.

When a platform should be fully built _in the pipeline_, wire it in here:

| Pipeline job | Add before `flutter build …` | Needs |
|---|---|---|
| Dockerfile `flutter-builder` **+** release `build-flutter-web` | `make web-crypto` (then drop the committed bundle and gitignore it) | Node in the image **and** `COPY packages/` into the build context (entry.mjs imports `../../packages/shared-ts`) |
| release `build-flutter-linux` / `build-flutter-macos` | `make native-crypto` + a CMake `install()` of the lib into the bundle | gcc/clang + `CGO_ENABLED=1` |
| **Android/iOS release job (does not exist yet)** | `make native-crypto-android` | `ANDROID_NDK_HOME` + an `apk`/`appbundle` build job |

> ⚠️ **When mobile becomes release-ready:** the Android/iOS builds will most
> likely live on **Codemagic** (ships the NDK/Xcode toolchains out of the box),
> not in this GitHub Actions pipeline. Wherever they run, the job must produce
> the native libs first — `make native-crypto-android` for the per-ABI jniLibs
> (or the iOS static-lib/xcframework path) — since they're gitignored and would
> otherwise ship **without** the crypto lib.
