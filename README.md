<p align="center"><img src="https://raw.githubusercontent.com/go-coff/brand/main/social/go-coff-pectl.png" alt="go-coff/pectl" width="720"></p>

# go-coff/pectl

[![CI](https://github.com/go-coff/pectl/actions/workflows/ci.yml/badge.svg)](https://github.com/go-coff/pectl/actions/workflows/ci.yml)
[![Coverage](https://img.shields.io/badge/coverage-100%25-brightgreen)](https://github.com/go-coff/pectl/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/go-coff/pectl.svg)](https://pkg.go.dev/github.com/go-coff/pectl)

Cobra-based reference CLI for building UEFI PE/COFF binaries in pure Go, built on
the [`github.com/go-coff/peln`](https://github.com/go-coff/peln) library. It
**links**, **converts**, **appends to** and **signs** PE32+/EFI images without
binutils, LLD, `objcopy` or `sbsign`.

## Install

```sh
go install github.com/go-coff/pectl@latest
```

Pre-built binaries for linux/macOS/windows are attached to each
[release](https://github.com/go-coff/pectl/releases).

## Subcommands

```text
pectl link     [flags] OBJECT...   # link relocatable .o files → PE32+ EFI
pectl link-pie [flags] PIE-ELF     # convert a position-independent ELF → PE32+ EFI
pectl objcopy  [flags] INPUT.elf   # ELF → flat binary / SREC / Intel HEX / U-Boot uImage
pectl append   [flags] INPUT       # add PE sections (UKI assembly)
pectl pack     [flags] INPUT.efi   # compress PE32+ EFI → self-extracting PE32+ EFI
pectl sign     [flags] INPUT       # Authenticode-sign for SecureBoot
```

### `pectl link`

Link one or more COFF/PE or ELF **relocatable** objects (e.g. from TinyGo or
clang) into a self-contained PE32+ EFI application — the pure-Go replacement for
`lld-link /subsystem:efi_application`, covering targets LLD's COFF driver does
not (riscv64, loongarch64). Machine is auto-detected from the first object.

```sh
pectl link --allow-unresolved -o BOOTAA64.EFI main-arm64.o thunk-arm64.o
```

Flags: `--machine` (amd64|arm64|riscv64|loongarch64), `--entry` (default
`_start`), `--subsystem` (default 10), `--image-base`, `--allow-unresolved`,
`--section-alignment`, `--file-alignment`, `--header-reserve`.

### `pectl link-pie`

Convert an already-linked **position-independent** ELF executable (`ET_DYN`, as
produced by `go build -buildmode=pie` for a bare-metal `GOOS` such as
**TamaGo**) into a PE32+/EFI application. Its `R_*_RELATIVE` dynamic
relocations are translated to PE base relocations so UEFI firmware rebases the
image at load; the entry point comes from `e_entry`.

```sh
# A TamaGo loong64 PIE → a UEFI application.
GOOS=tamago GOARCH=loong64 go build -buildmode=pie -o hello-pie.elf .
pectl link-pie -o BOOTLOONG64.EFI hello-pie.elf
```

Flags: `--subsystem` (default 10), `--image-base` (default: lowest segment
vaddr floored to 64 KiB), `--section-alignment`, `--file-alignment`. Supported
machines: amd64, arm64, riscv64, loongarch64.

### `pectl objcopy`

Convert an ELF executable into a **non-UEFI** bare-metal image — a flat
binary (like `objcopy -O binary`), a Motorola S-record, an Intel HEX file,
or a legacy U-Boot uImage (like `mkimage`) — in pure Go. For targets loaded
at a fixed address rather than via UEFI (a Raspberry Pi `kernel.img`, a
QEMU `-kernel` image, a flash image, a `bootm` payload).

```sh
pectl objcopy -O binary -o kernel.img kernel.elf
pectl objcopy -O ihex   -o fw.hex     fw.elf
pectl objcopy -O uimage --load 0x80000 --entry 0x80000 --name linux -o uImage kernel.elf
```

`-O`/`--output-target` selects `binary` (default), `srec`, `ihex` or
`uimage`. Addresses default to the image base / ELF `e_entry`; override with
`--load`/`--entry`. The uImage arch is taken from the ELF unless
`--uimage-arch` is given.

### `pectl append`

Add sections at the end of an existing PE32/PE32+ image while preserving every
existing section byte-for-byte — UEFI Unified Kernel Image (UKI) assembly.

```sh
pectl append --linux=vmlinuz.efi \
             --initrd=initramfs.cpio.gz \
             --cmdline=cmdline \
             --osrel=os-release \
             --uname=uname \
             -o BOOTAA64.EFI stub.efi
```

| Flag        | Section    | Typical contents                                     |
| ----------- | ---------- | ---------------------------------------------------- |
| `--linux`   | `.linux`   | kernel image (`vmlinuz` / `vmlinuz.efi` / `bzImage`) |
| `--initrd`  | `.initrd`  | initramfs (`initramfs.cpio.gz`)                      |
| `--cmdline` | `.cmdline` | kernel command line                                  |
| `--osrel`   | `.osrel`   | `os-release` metadata                                |
| `--uname`   | `.uname`   | `uname -r` string                                    |

For any other section: `--section name=path` (also `-s name=path`), repeatable,
any name up to 8 bytes. The legacy `--add-section name=path` flag is still
accepted (hidden) for older scripts.

### `pectl pack`

Compress a PE32+/EFI image into a **self-extracting PE32+/EFI envelope**
(decompressor stub + compressed `.payload` section) — the UPX-equivalent
that does not exist anywhere else for this format. Designed for
cloud-boot's M6.2 milestone (mitigates the EDK2 OVMF
`CpuPageTableLib` `#GP` on `LoadImage` of sufficiently large EFI
binaries). Architecture is auto-detected from the input's COFF
`Machine` field.

```sh
# Default: flate at compress/flate.DefaultCompression.
pectl pack -o BOOTAA64-packed.EFI BOOTAA64.EFI
# pack: BOOTAA64.EFI (3324928 bytes) -> BOOTAA64-packed.EFI (2851840 bytes, 14.2% reduction, compressor=flate)

# Maximum compression, explicit codec name.
pectl pack -c flate --level 9 -o out.efi in.efi
```

Flags: `-c`/`--compressor` (`flate` (default) | `lzfse` | `lz4`),
`--level` (default `-1` = `compress/flate.DefaultCompression`),
`-o`/`--output` (required).

- `flate`: stdlib `compress/flate`, runnable envelope.
- `lzfse`: **host-side only** as of M6.2 PR4. `pectl pack -c lzfse`
  produces a packed PE on disk and prints a WARNING that the embedded
  runtime decompressor stub is still flate-only — the resulting EFI
  will NOT boot under firmware until LZFSE-aware stubs ship (deferred
  follow-up). Use `-c flate` for a runnable packed EFI.
- `lz4`: still returns "compressor not implemented in this build".

Supported architectures: arm64, riscv64, loong64 (runnable envelopes);
amd64 ships the same wire format but its runtime stub is deferred to
the `m6-2-pr2-amd64-wip` branch.

### `pectl sign`

Authenticode-sign a PE/COFF image so SecureBoot firmware whose db trusts the
cert will load it — pure Go, no `sbsign`/`sbsigntools`.

```sh
pectl sign --key=db.key --cert=db.crt -o BOOTAA64-signed.EFI BOOTAA64.EFI
```

Sign **last** — any post-signing `append` invalidates the Authenticode digest.

## Exit codes

| Code | Meaning |
| ---- | ------- |
| `0`  | Success |
| `1`  | Runtime error (couldn't read an input, the library rejected it, couldn't write the output) |
| `2`  | Usage error (missing/unknown flag, bad argument, wrong arg count) |

## See also

- [`github.com/go-coff/peln`](https://github.com/go-coff/peln) — the underlying
  library (`linker` + `appender`); embed it directly instead of shelling out.
- [`github.com/usbarmory/tamago`](https://github.com/usbarmory/tamago) — the
  bare-metal Go runtime whose `-buildmode=pie` output `link-pie` consumes.

## License

[BSD 3-Clause](LICENSE).
