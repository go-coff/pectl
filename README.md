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
pectl append   [flags] INPUT       # add PE sections (UKI assembly)
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
