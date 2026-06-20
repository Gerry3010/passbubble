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

```bash
./build.sh            # host desktop (.so/.dylib/.dll) → passbubble_crypto/build/
./build.sh android    # all Android ABIs → android/app/src/main/jniLibs/<abi>/  (needs ANDROID_NDK_HOME)
```

### Per-platform bundling

| Platform | Artifact | How it's loaded / bundled |
|----------|----------|---------------------------|
| Android  | `lib*.so` per ABI in `android/app/src/main/jniLibs/` | packaged automatically; `DynamicLibrary.open('libpassbubble_crypto.so')` |
| Linux    | `lib*.so` | copy into the bundle via `linux/CMakeLists.txt` `install(...)`; `DynamicLibrary.open('libpassbubble_crypto.so')` |
| Windows  | `*.dll`  | copy next to the runner via `windows/CMakeLists.txt`; `DynamicLibrary.open('passbubble_crypto.dll')` |
| macOS    | static `.a` / xcframework | link in the Runner target; `DynamicLibrary.process()` |
| iOS      | static `.a` (`go build -buildmode=c-archive`, per-arch, `lipo`/xcframework) | link in the Runner target; `DynamicLibrary.process()` |

> CI builds these with the platform SDKs (NDK / Xcode / MSVC) and drops them in
> the locations above before `flutter build`. Locally, `./build.sh` covers the
> host so `flutter test` and desktop runs work. Tests point at the built `.so`
> via `mlKemLibraryPathOverride` / `PASSBUBBLE_CRYPTO_LIB`.
