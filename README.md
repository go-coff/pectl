# go-coff/pec

[![CI](https://github.com/go-coff/pec/actions/workflows/ci.yml/badge.svg)](https://github.com/go-coff/pec/actions/workflows/ci.yml)
[![Coverage](https://img.shields.io/badge/coverage-100%25-brightgreen)](https://github.com/go-coff/pec/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/go-coff/pec.svg)](https://pkg.go.dev/github.com/go-coff/pec)

Reference CLI built on top of the [`github.com/go-coff/pe`](https://github.com/go-coff/pe)
library. `pec` is a minimal stand-in for `objcopy --add-section`, restricted to
image-section appending — the case that matters for UEFI Unified Kernel Image
(UKI) assembly.

## Install

```sh
go install github.com/go-coff/pec@latest
```

## Usage

```sh
pec \
    --add-section .osrel=os-release \
    --add-section .cmdline=cmdline \
    --add-section .linux=vmlinuz \
    --add-section .initrd=initramfs.cpio.gz \
    -o BOOTX64.EFI linuxx64.efi.stub
```

Each `--add-section name=path` appends the file at `path` as a new PE section
named `name`, with `pe.DefaultCharacteristics` (read-only initialised data).
Existing sections in the input image are preserved byte-for-byte.

## See also

- [`github.com/go-coff/pe`](https://github.com/go-coff/pe) — the underlying
  library; use it directly if you want to embed PE-section appending in your
  own tool rather than shell out to a binary.

## License

[BSD 3-Clause](LICENSE).
