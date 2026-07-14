# MiSTer ao486 DOS Toolkit

A set of command-line tools for building, converting, and launching DOS and Windows 3.1 games on the MiSTer's ao486 core.

---

## Command-Line Tools

### `mkvhd`
Creates a bootable hard disk image (VHD) for the ao486 core, optionally auto-installing a game archive onto it. Supports both plain MS-DOS 6.22 games and Windows 3.1 games.

```
mkvhd <name.vhd>
mkvhd -dos <name.vhd>
mkvhd -dos <name.vhd> <archive>
mkvhd -win31 <name.vhd>
mkvhd -win31 <name.vhd> <archive>
```

- `mkvhd <name.vhd>` — creates a blank, unformatted VHD of a size you specify.
- `mkvhd -dos <name.vhd>` — creates a VHD and automatically installs MS-DOS 6.22 onto it (boots real DOS in QEMU to do the formatting/copying).
- `mkvhd -dos <name.vhd> <archive>` — same as above, then extracts `<archive>` onto the VHD into its own folder, offers to let you pick which extracted file should auto-launch on boot, and adds it to `AUTOEXEC.BAT`.
- `mkvhd -win31 <name.vhd>` — creates a VHD using a pre-built Windows 3.1 template (no emulated boot required; just copies the template content onto a fresh container). Requires `win31_template.vhd` to be in place (see File Placement below).
- `mkvhd -win31 <name.vhd> <archive>` — same as above, then extracts `<archive>` onto the VHD into its own subfolder, and configures Windows 3.1 to auto-launch the game's main executable via `WIN.INI`'s `run=` line on startup.

Supports `.zip`, `.tar`, `.tar.gz`/`.tgz`, `.tar.bz2`/`.tbz2` archives. When an archive is given, `mkvhd` suggests an appropriately-sized VHD automatically. Supports sizes from 2MB up to 2047MB (the real ceiling for DOS-compatible FAT16).

**Bulk file copying** (the template copy and game archive injection steps) is done via a kernel loop-mount + `cp -r` rather than `mtools`. This is intentional: `mtools`' recursive `mcopy -s` was found to silently drop entire files and subdirectories on real, large archives — confirmed in practice on actual game content. The loop-mount path is reliable, at the cost of needing root for those specific steps. Targeted `mtools` operations (system files, attribute patching) are still used where they're appropriate. During large copies, `mkvhd` shows a live progress counter (`Copying files: 247/1265 (19%)`) so you can see it's working rather than staring at a silent terminal for a couple of minutes.

**DOS attribute preservation**: when injecting archives, `mkvhd` correctly restores DOS hidden/system/read-only attributes from the source — but only when the archive was genuinely made on a DOS/FAT host. Archives zipped on Unix/macOS/Windows tooling (the majority of abandonware downloads) carry Unix permission bits in that field instead; `mkvhd` detects this and skips attribute restoration for those files, leaving them with a clean Archive attribute rather than incorrectly flagging them as hidden or system. If you've ever had injected game files show up invisible in Windows 3.1's File Manager, this is the fix.

### `resizevhd`
Resizes an existing, already-configured game VHD — larger or smaller — while preserving every file on it exactly as-is: system files, game files, `AUTOEXEC.BAT`, `CONFIG.SYS`, save data, all of it, byte-for-byte.

```
resizevhd <name.vhd>
```

Reports the VHD's current file size and actual content used, then suggests a new size (same 25%-headroom logic `mkvhd` uses). You can accept the suggestion, type your own size, or cancel.

If you proceed, you're asked whether to replace the original file. Answering yes deletes the original; answering no keeps it as a timestamped backup. Either way, the resized file ends up with the *same filename* as the original, so any `.mgl` launcher pointing at it keeps working without changes.

Like `mkvhd`, the bulk file-copy step uses a kernel loop-mount + `cp -r` for reliability, not `mtools`. The same `mtools` reliability issue (silent data loss on large copies) applies here too, since `resizevhd` copies a whole VHD's worth of real content. DOS attribute bytes (hidden/system/read-only) are captured from the source before the copy and faithfully restored on the new container afterward. The boot sector's code bytes are also preserved from the source (only the BPB geometry fields are rewritten for the new size), ensuring the resized VHD boots with the exact same DOS version it started with.

### `mkmgl`
Generates a `.mgl` launcher file for an ao486 game, placing it in the correct MiSTer menu folder so the game appears under the right category in the OSD.

```
mkmgl -dos "Display Name" [source_folder]
mkmgl -win31 "Display Name" [source_folder]
```

- `-dos` — places the `.mgl` in `/media/fat/_DOS Games`
- `-win31` — places the `.mgl` in `/media/fat/_Win 3.1 Games`

`source_folder` defaults to the current directory if omitted. If the folder contains multiple `.vhd` files, `mkmgl` lists them and asks which one to use. The path written into the `.mgl` is always relative to `/media/fat/games/AO486/`, which is what the ao486 core resolves paths against.

### `mkchd`
Converts a CD image into the compressed `.chd` format ao486 uses for CD-ROM games.

```
mkchd <input.iso|.cue|.bin|.gdi> <output.chd>
```

Wraps `chdman` with safe defaults, checks for a clean conversion, and offers to delete the original source file(s) afterward (including every file a `.cue` sheet references, not just the `.cue` itself).

### `mkima`
Creates a raw floppy disk image (`.ima` — plain FAT12, no partition table, exactly like a real floppy), optionally injecting files from a directory or an archive.

```
mkima <name.ima>
mkima <name.ima> -s <size>
mkima <name.ima> <source>
mkima <name.ima> <source> -s <size>
```

- `mkima <name.ima>` — creates a blank, formatted floppy image (1.44MB by default).
- `mkima <name.ima> -s <size>` — same, at an explicit size.
- `mkima <name.ima> <source>` — formats a floppy and copies `<source>`'s files onto it. `<source>` can be a directory or an archive (`.zip`, `.tar`, `.tar.gz`/`.tgz`, `.tar.bz2`/`.tbz2`).
- `mkima <name.ima> <source> -s <size>` — same, with an explicit size.

Sizes (`-s`): `360k`, `720k`, `1.2m`, `1.44m` (default), `2.88m`. When a `<source>` is given and no `-s` is specified, `mkima` automatically picks the smallest standard size that fits.

Entirely root-free: both formatting and file injection go through `mtools` directly on the image file.

### `mountvhd` / `umountvhd`
Mounts a VHD file so you can browse or edit its contents directly.

```
. mountvhd <name.vhd>
```

Note the leading `. ` (dot-space) — this *sources* the script into your current shell, which is required for the automatic `cd` into the mounted volume to take effect. Running it normally will still mount the VHD correctly; it just won't move you into it.

When done:

```
. umountvhd
```

Also sourced — it `cd`s back out first (Linux won't unmount a filesystem while your shell is inside it), then unmounts and removes the empty mount-point.

### `mountchd` / `umountchd`
Same idea, but for `.chd` files created by `mkchd`.

```
. mountchd <name.chd>
```

Extracts the disc with `chdman` (with live progress), detects sector format, strips raw-sector wrappers if needed to produce a mountable ISO, then mounts and drops you in. The extracted copy lives on `/media/fat` rather than `/tmp` since large discs could exhaust RAM-backed scratch space.

When done:

```
. umountchd
```

Unmounts and deletes the extracted scratch copy.

Only supports CHDs created by `mkchd` (CD images via `createcd`).

---

## File Placement

### Command-line tools
| File | Install to |
|---|---|
| `mkvhd` | `/media/fat/linux/mkvhd` |
| `resizevhd` | `/media/fat/linux/resizevhd` |
| `mkmgl` | `/media/fat/linux/mkmgl` |
| `mkchd` | `/media/fat/linux/mkchd` |
| `mkima` | `/media/fat/linux/mkima` |
| `mountvhd` | `/media/fat/linux/mountvhd` |
| `umountvhd` | `/media/fat/linux/umountvhd` |
| `mountchd` | `/media/fat/linux/mountchd` |
| `umountchd` | `/media/fat/linux/umountchd` |

All nine need executable permission:
```
chmod +x /media/fat/linux/mkvhd /media/fat/linux/resizevhd /media/fat/linux/mkmgl \
         /media/fat/linux/mkchd /media/fat/linux/mkima \
         /media/fat/linux/mountvhd /media/fat/linux/umountvhd \
         /media/fat/linux/mountchd /media/fat/linux/umountchd
```

### Helper files (used internally — don't run these directly)
| File | Install to |
|---|---|
| `make_mbr.py` | `/media/fat/linux/make_mbr.py` |
| `fix_bpb.py` | `/media/fat/linux/fix_bpb.py` |
| `newgame_controller.py` | `/media/fat/linux/newgame_controller.py` |
| `mbr_bootstrap.bin` | `/media/fat/linux/mbr_bootstrap.bin` |
| `qemu_autoexec.bat` | `/media/fat/linux/qemu_autoexec.bat` |

### Compiled binaries
| File | Install to |
|---|---|
| `chdman` | `/media/fat/linux/chdman` |
| `qemu-system-i386` | `/media/fat/linus/qemu-system-i386` |
| `qemu-bios/` (whole folder) | `/media/fat/linux/qemu-bios/` |
| `mtools` | `/media/fat/linux/mtools` |

Binaries need executable permission too:
```
chmod +x /media/fat/linux/chdman /media/fat/linux/qemu-system-i386 /media/fat/linux/mtools
```

`mtools` is used internally by `mkima` and for targeted operations in `mkvhd`/`resizevhd` (attribute patching, individual system files). It dispatches behavior based on the filename it's invoked as (e.g. `mcopy`, `mformat`, `mattrib`), so it needs same-directory symlinks — these are created automatically on first use.

### DOS and Windows 3.1 system files
| File | Install to |
|---|---|
| `disk1.img` | `/media/fat/games/ao486/floppy/DOS622/disk1.img` |
| `dos_template.vhd` | `/media/fat/games/ao486/dos_template.vhd` |
| `win31_template.vhd` | `/media/fat/games/ao486/win31_template.vhd` |

`disk1.img` and `dos_template.vhd` are genuine Microsoft MS-DOS 6.22 files and must come from your own legally-owned copy.

`dos_template.vhd` is a small VHD holding the DOS/MISTER/DRIVERS/UTIL folders that get copied onto every new `-dos` game VHD. It is never itself booted — only ever mounted as a file source.

`win31_template.vhd` is required for `mkvhd -win31`. It must be a complete, bootable Windows 3.1 installation on a FAT16 VHD. `mkvhd -win31` copies its contents onto a fresh container of the correct size and then injects the game archive on top; it never modifies the template itself. Build this once from a clean Windows 3.1 install. The template VHD must use the same MS-DOS version internally as your `dos_template.vhd` (the boot sector code is preserved from the template, so mismatched DOS versions will produce a VHD that looks valid but fails to boot).

---

Once everything above is in place, `mkvhd -dos` and `mkvhd -win31` are ready to use immediately. None of these tools depend on the MiSTer's root filesystem being writable — everything lives on `/media/fat` (persistent storage) or `/tmp` (RAM-backed scratch space).
