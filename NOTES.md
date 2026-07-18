# aotools

Build and development notes for aotools — a single-binary Go port of the MiSTer ao486 DOS Toolkit (`mkvhd`, `resizevhd`, `mkmgl`, `mkchd`, `mkima`, `mountvhd`/`umountvhd`, `mountchd`/`umountchd`, and the `make_mbr.py`/`fix_bpb.py`/`newgame_controller.py` helpers behind them). For end-user install and command syntax, see `README.md`. This file documents what's been tested, the bugs found along the way and how each was fixed, and how to rebuild from source.

Ported directly from the toolkit's own shell/Python scripts, not reconstructed from documentation. Deployed at `/media/fat/linux/aotools/aotools`, alongside — not replacing — the original toolkit files.

The shell integration (`mountvhd`/`umountvhd`/`mountchd`/`umountchd`/`mkvhd`/`resizevhd`/`mkmgl`/`mkchd`/`mkima`) that used to live in a separate `aotools-functions.sh` is now embedded in the binary itself and wired into the shell by a one-time `aotools install` — no second file to source or keep in sync.

## What's here

- `*.go`, `go.mod` — full source, single `package main`, no dependencies outside the Go standard library
- This file

## One-time setup

```
/media/fat/linux/aotools/aotools install
```

This appends a marker-delimited block to `/media/fat/linux/user-startup.sh` (MiSTer's persistent boot hook):

```
# --- aotools:begin (shell functions + PATH) ---
cat > /etc/profile.d/aotools.sh <<'AOTOOLS_PROFILE_EOF'
export PATH="$PATH:/media/fat/linux/aotools"
eval "$(/media/fat/linux/aotools/aotools shellinit)"
AOTOOLS_PROFILE_EOF
# --- aotools:end ---
```

`user-startup.sh` runs once as a standalone child process at boot; anything it exports or evals directly dies with that process and never reaches a login shell opened afterward. `/etc/profile`, which MiSTer's login shells do source, loops over every `*.sh` file under `/etc/profile.d/` — so instead of exporting anything itself, the block above has `user-startup.sh` (re)write `/etc/profile.d/aotools.sh` on every boot, with the real `export`/`eval` lines inside it. `install` also writes that file immediately, so the effect is live without a reboot. See bug 5 below for why this indirection is necessary.

`install` is idempotent — it checks for its own marker before appending, so running it again is a no-op — and only ever appends to `user-startup.sh`; nothing else in that file is touched. Once run, `aotools` is on `$PATH` for any new shell, and `mountvhd`/`umountvhd`/`mountchd`/`umountchd`/`mkvhd`/`resizevhd`/`mkmgl`/`mkchd`/`mkima` are available directly, no sourcing required.

A machine running an older install (shell functions only, no PATH; or the broken direct-export version) is upgraded automatically the next time `install` runs — it detects the old block and replaces it.

To pick up both in the current shell without opening a new session:

```
export PATH="$PATH:/media/fat/linux/aotools"
eval "$(/media/fat/linux/aotools/aotools shellinit)"
```

## Usage

```
aotools create vhd -dos mygame.vhd game.zip
aotools create mgl -dos "My Game" .
```

## Command reference

```
aotools create vhd [-dos|-win31] <name.vhd> [archive|folder]
aotools resize vhd <name.vhd>
aotools create mgl -dos|-win31 "Display Name" [source_folder]
aotools create chd <input.iso|.cue|.bin|.gdi> <output.chd>
aotools create diskimage <name.ima> [source] [-s size]
aotools mount vhd <name.vhd>            (or `mountvhd <name.vhd>`)
aotools umount vhd                      (or `umountvhd`)
aotools mount chd <name.chd>            (or `mountchd <name.chd>`)
aotools umount chd                      (or `umountchd`)
aotools mount diskimage <name.ima>
aotools umount diskimage
aotools install                         one-time: wire shell functions + PATH into every future shell
aotools uninstall                       reverses install (removes the user-startup.sh wiring only)
aotools shellinit                       prints the shell functions; used internally by install
aotools doctor                          reports on qemu/chdman/templates/mtools/etc. -- see README.md
```

Every command follows `<verb> <noun>`: `create`/`resize`/`mount`/`umount`, followed by what it acts on (`vhd`/`mgl`/`chd`/`diskimage`). See the CLI restructuring section below for the earlier, inconsistent layout this replaced.

## VHD sizing and partitioning

Every VHD `create vhd`/`resize vhd` produces uses the same fixed layout: a standard DOS MBR with a single partition entry starting at LBA 128 (16 heads / 63 sectors-per-track legacy CHS geometry), followed by a FAT12 or FAT16 volume filling the rest of the file. This mirrors the original `make_mbr.py`/`fix_bpb.py` layout exactly (`mbr.go`'s `makeMBR`/`fixBPB` are direct ports), and every path that writes a VHD — blank, `-dos`, `-win31`, or a resize — goes through it.

**Size limits.** 2MB is the practical floor for anything auto-formatted with DOS (`-dos`/`-win31`): MS-DOS 6.22's own system files consume close to 1MB by themselves, leaving nothing for game data below that. 2047MB is the hard ceiling: FAT16 tops out there even with the largest standard (32KB) cluster size, and MS-DOS 6.22 has no FAT32 support at all, so there's no way past it without breaking DOS compatibility. A blank VHD (no `-dos`/`-win31`) isn't bound by the 2MB floor, since nothing auto-formats it.

**Cluster size and FAT12 vs. FAT16** (`clusterSectorsFor`, `vhdcreate.go`) are chosen by size bracket, not left to `mkfs.vfat`'s own defaults:

```
   < 16MB  ->  1 sector/cluster  (512B),  FAT12/16 auto-selected by mkfs.vfat
  <= 128MB  ->  4 sectors/cluster (2KB),  FAT16 forced
  <= 256MB  ->  8 sectors/cluster (4KB),  FAT16 forced
  <= 512MB  -> 16 sectors/cluster (8KB),  FAT16 forced
  <= 1024MB -> 32 sectors/cluster (16KB), FAT16 forced
   > 1024MB -> 64 sectors/cluster (32KB), FAT16 forced
```

Below 16MB, `mkfs.vfat` is left to pick FAT12 vs. FAT16 on its own — forcing `-F 16` at that size produced "General failure reading drive C" on real DOS. The root cause, found during the original bash toolkit's own development: FAT type is determined purely by cluster count (below 4085 clusters is FAT12, 4085-65524 is FAT16), and forcing FAT16 on a small partition can land just under that boundary — an 8MB partition at 2KB clusters yields only ~4064 clusters, so `mkfs.vfat` built FAT16 structures while DOS's own driver read the cluster count and correctly decided FAT12, then misread every FAT entry's width, corrupting the volume (`mkfs.vfat`'s own warning, "Not enough clusters for a 16 bit FAT! ... misinterpreted as having a 12 bit FAT," confirmed the mechanism directly). At 16MB and above, `mkfs.vfat`'s own heuristic switches to FAT32 well before reaching the largest size this toolkit supports, unless `-F 16` is passed explicitly; since MS-DOS 6.22 can't mount FAT32 at all, every bracket from 16MB up forces FAT16 outright. These brackets were verified empirically against real `mkfs.vfat`/DOS behavior, not derived from documentation alone.

**Formatting and BPB repair** (`formatAndFixBPB`, `vhdcreate.go`; `makeMBR`/`fixBPB`, `mbr.go`) happens in a fixed sequence: `makeMBR` writes the MBR and a provisional partition-table entry (type `0x06`, corrected later) sized to the target file; a loop device is attached at the partition offset and `mkfs.vfat` formats it with the bracket-appropriate cluster size; then `fixBPB` rewrites the geometry-dependent BPB fields `mkfs.vfat` can't know about on its own — 16-bit or 32-bit total sector count (whichever fits), hidden-sector count (the partition's LBA start), heads, and sectors-per-track — and corrects the MBR partition-type byte to `0x01` (FAT12), `0x04` (FAT16, ≤65535 sectors), or `0x06` (FAT16, >65535 sectors) based on what `mkfs.vfat` actually produced.

**Boot sector code is never fabricated.** `writeBootSectorMerge` (`mbr.go`) keeps the freshly-written BPB (bytes 11-61 of the boot sector) but overwrites the jump instruction/OEM name (bytes 0-10) and the actual boot code plus signature (bytes 62-511) with the real bytes from a working DOS source — either the template being cloned (`-win31`, `resize vhd`) or QEMU's own FORMAT/SYS output (`-dos`). Real MS-DOS boot code is tightly paired with the specific `IO.SYS`/`MSDOS.SYS` build it shipped with; a synthesized boot sector can look structurally correct — right BPB, right signature — and still hang on real ao486 hardware. `readFatAttrManifest`/`mattribFlagsFor` (`mbr.go`) do the same kind of preservation for DOS attribute bits (read-only/hidden/system), which a plain Unix file copy always drops.

**Growing an existing VHD** (`resize vhd`, `vhdresize.go`) reads the whole source into staging, suggests a new size as actual content plus 25% headroom (minimum 100KB), builds a fresh container through the same `formatAndFixBPB` path at the chosen size, copies everything back — `IO.SYS`/`MSDOS.SYS`/`COMMAND.COM` first and in that fixed order, for the reasons in bug 7 below — merges in the original boot sector, and verifies the result byte-for-byte against the original before offering to replace it (or keep it alongside as a timestamped backup). `create vhd -win31` (`createWin31VHD`) sizes itself the same way against its template plus any injected game content, with a larger 1MB minimum headroom to leave Windows 3.1 itself some working room.

**`create diskimage` (`.ima`) is a different, simpler format entirely.** Floppy images are raw "superfloppy" media with no partition table and no MBR — `cmdIMACreate` (`ima.go`) runs `mkfs.vfat -I` directly against the whole file. Sizes are restricted to the five real floppy geometries DOS recognizes (360k, 720k, 1.2m, 1.44m, 2.88m); there's no arbitrary sizing or cluster-size selection to make, since each of those sizes has one standard, fixed FAT12 layout. `mount diskimage` mirrors this: a plain `mount -o loop` with no partition offset, unlike `mount vhd`'s offset-128-sectors mount.

## Building from source

The binary at `/media/fat/linux/aotools/aotools` is a static ARMv7 executable — no compiler needed to run it, only to modify it.

On-device build (MiSTer has no compiler by default but does have network access):

```
curl -sSkL -o /tmp/go.tar.gz https://go.dev/dl/go1.26.5.linux-armv6l.tar.gz
mkdir -p /media/fat/linux/.mistercli_buildtools/goroot
tar -C /media/fat/linux/.mistercli_buildtools/goroot -xzf /tmp/go.tar.gz --strip-components=1
export GOROOT=/media/fat/linux/.mistercli_buildtools/goroot
export PATH=$GOROOT/bin:$PATH
export GOCACHE=/media/fat/linux/.mistercli_build/.gocache
export GOPATH=/media/fat/linux/.mistercli_build/.gopath
cd /media/fat/linux/.mistercli_build
go build -ldflags="-s -w" -o aotools .
```

Directory names still read `mistercli`, a holdover from before this tool was renamed to `aotools` — harmless, build-cache paths only, not user-facing.

Set `GOROOT`, `PATH`, `GOCACHE`, and `GOPATH` as separate `export` statements, not chained on one line — bash expands `$GOROOT` against its current value before that line's own assignments take effect, so a combined line picks up a stale value.

Extract the toolchain and point `GOCACHE`/`GOPATH` at `/media/fat`, not `/tmp` — `/tmp` is a ~240MB tmpfs and can't hold a full Go toolchain plus build cache.

First build compiles the standard library (a few minutes on ARMv7); subsequent builds are seconds.

Cross-compiling from a workstation:
```
GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 go build -ldflags="-s -w" -o aotools .
```

## External dependencies and their origin

None of `qemu-system-i386`, `chdman`, or `mtools` are built by aotools itself, and none are bundled inside the aotools binary. All three, though, are purpose-built for this project rather than generic pre-built binaries: each was cross-compiled, or built and verified from scratch, specifically for MiSTer's ARM target.

`qemu-system-i386` is a cross-compiled, ARM-emulation-validated static binary confirmed running on real hardware, and is the foundation of the headless DOS-install pipeline behind `create vhd -dos`/`-win31`. See the build notes below.

`chdman` is cross-compiled from MAME's own source for MiSTer's ARM target. Its build history includes a real crash that was found, initially misdiagnosed, and eventually correctly root-caused. See the build notes below.

`mtools` is a separately built static ARM binary, verified independently of the other two, with its own build-time bug traced to a glibc limitation. See the build notes below.

All three, plus the qemu-bios directory, are fetched via `aotools install`'s dependency-download flow (`fetch.go`'s `offerFetchMissing`/`downloadFile`) from a single GitHub repository if missing locally:

```
mtools            -> https://raw.githubusercontent.com/fry-guy/mister-ao486-command-line-tools-cli/main/mtools
chdman            -> https://raw.githubusercontent.com/fry-guy/mister-ao486-command-line-tools-cli/main/chdman
qemu-system-i386  -> https://raw.githubusercontent.com/fry-guy/mister-ao486-command-line-tools-cli/main/qemu-system-i386
qemu-bios/*       -> same repo's qemu-bios/ directory (ships alongside qemu-system-i386, not a separate build)
```

This repository is where these binaries were published after being built during this project's own development, for the wider MiSTer community to use directly rather than everyone repeating the cross-compile work. `depcheck.go`'s own comment reflects this directly rather than describing them as generic pre-compiled binaries.

`aotools install`'s download flow fetches over HTTPS via `curl -k`, since this MiSTer's buildroot has no CA certificate bundle installed — the same tradeoff the Go toolchain download in the build instructions below makes. Nothing is compiled on-device by aotools itself; embedding these binaries inside the aotools binary would both bloat it and raise licensing questions this project has no standing to answer.

The DOS and Windows 3.1 VHD templates and the boot floppy (`dos_template.vhd`, `win31_template.vhd`, `disk1.img`) are fetched the same way, from archive.org, for a related but distinct reason: they contain actual copyrighted Microsoft system files, not something any tool has the right to bundle and redistribute.

## External dependency build notes

### qemu-system-i386

Cross-compiled as a static ARM binary (QEMU 8.2.0), validated under ARM emulation, and confirmed running on real hardware via `qemu-system-i386 --version`. No significant bugs surfaced in the build itself; the harder problem was driving it headlessly through a DOS install, handled by `newgame_controller.py` and the embedded MBR bootstrap described under "Inherited from the original bash toolkit" below.

### chdman

Cross-compiled from MAME's own chdman source for MiSTer's ARM target, requiring a hand-written `version.cpp` (normally auto-generated by MAME's own build tooling) and an SDL2 clipboard stub to satisfy symbols the cross-compile toolchain didn't provide.

The build's worst bug: `createcd`/`createraw` crashed with SIGBUS whenever the LZMA codec (`cdlz`, chdman's own default) was used, traced to a stale object file compiled under different NEON codegen flags that `make`'s incremental build never recompiled once those flags changed elsewhere in the build. A fully clean rebuild removed the crash entirely.

The fix has been verified across every codec, including a production run of `chdman createcd -c cdlz` against a real disc image with a full SHA1 verification pass. `mkchd` and aotools' own `chd.go` still carry a defensive SIGBUS-detection safeguard (exit code 135 = 128+SIGBUS, deletes the corrupt partial `.chd`) as a fallback, though it shouldn't fire in normal use.

### mtools

Built and verified as a static ARM binary (672KB stripped) in its own feasibility effort, separate from the qemu/chdman cross-compiles.

Its build bug: `iconv`'s codepage tables don't load reliably from a statically linked binary — a known glibc limitation, not ARM-specific — which broke filename codepage conversion ("Error converting to codepage 850") until `HAVE_ICONV_H` was disabled at build time in favor of mtools' own built-in codepage handling. Long-filename handling (8.3 short names alongside the full long name) was confirmed unaffected by the change.

See "Inherited from the original bash toolkit" below for the symlink-dispatch behavior and the two argument-semantics bugs found once mtools was actually integrated into the toolkit.

## Hardware test coverage

Everything below ran end-to-end against real hardware, not just through the compiler. Command names in the earlier entries predate the verb-first restructuring; see that section for the rename.

- `vhd create`: blank, `-dos` (full QEMU-driven FORMAT+XCOPY install), `-win31` (template clone with boot sector preservation), and `-dos` with a zip archive injected (wrapper-folder flattening, launch-executable picker, AUTOEXEC.BAT update)
- `vhd resize`: 4MB → 8MB, verified byte-for-byte against the original across three runs
- `mgl create`
- `ima create`: blank, sized, and with a source directory injected
- `mount vhd` / `umount vhd`
- `mount chd` / `umount chd`, against a real Simon the Sorcerer II CD image (`.cue`/`.bin` → mounted, game files visible)
- `install` / `shellinit`: idempotency (running `install` twice appends the wiring once) and the full `eval "$(aotools shellinit)"` → `mountvhd` → cd in → `umountvhd` → cd back flow against a real DOS-formatted VHD
- `doctor`: reports `[ok]` for every dependency on a device that already has the original toolkit set up
- `chd create` end-to-end against a real CD image (`SIMON2.CUE`/`SIMON2.BIN`, ~407MB raw), producing a valid 177MB `.chd` (confirmed via `chdman info`), itself mounted successfully via `mount chd` — the full create-then-mount round trip, not just mounting a CHD built elsewhere
- `install`'s dependency-download offer: exercised by relocating real dependencies (`mtools`, `dos_template.vhd`) aside and running the full flow — declined (clean no-op), then accepted (each download lands at the right path with the right permissions and matches the original byte-for-byte) — before restoring the originals. The `dos_template.vhd` download link was corrected during this pass; it had pointed at the same archive.org file as `win31_template.vhd`
- `uninstall`: against a backed-up copy of the live `user-startup.sh` — confirmed it removes exactly the block `install` added and nothing else, that `install` re-adds it cleanly afterward, and that a second `uninstall` reports "Not installed" instead of erroring
- `install`/PATH, first pass: verified via a `bash -c` subshell of an already-live SSH session — a flawed test, since that subshell inherits its parent's already-exported environment rather than proving a genuinely new login sees it. See bug 5.
- `nano` wrapper's automatic DOS line-ending conversion: verified by mounting a VHD, writing a plain-LF test file inside the mount, invoking the wrapper with stdin redirected from `/dev/null` (nano itself exits immediately for lack of a terminal, but the wrapper's post-processing runs regardless of nano's exit code), and confirming the file came out as CRLF with the conversion message printed. Genuine interactive keystrokes in nano are untested here, but that code path is the unmodified real binary and carries no risk from this change.

## CLI restructuring (verb-first order) and disk image mounting

The original syntax put the format before the action (`vhd create`, `vhd resize`, `mgl create`, `chd create`, `ima create`), inconsistent with `mount`/`umount`, which were already action-first. All commands now put the verb first: `create vhd`, `resize vhd`, `create mgl`, `create chd`, `create diskimage`. `ima create` was renamed to `create diskimage` to match the more descriptive naming already used elsewhere; internal identifiers (`cmdIMACreate`, `imaMountPoint`, etc.) were left as-is, since only the user-facing command name changed.

`create diskimage` gained a size-selection prompt: a numbered list of the five standard floppy sizes (360k/720k/1.2m/1.44m/2.88m), 1.44m as the Enter-key default, and — when a source is given — the smallest size it fits marked "recommended for source." An explicit `-s <size>` flag skips the prompt. Both zip archives and plain folders were already supported as sources through the same `isDir`/`extractArchive` branching `create vhd`/`create mgl` use, so no new source-type handling was needed.

`mount diskimage <name.ima>` / `umount diskimage` were added, matching `mount vhd`/`mount chd` exactly: auto-cd on mount, auto-cd back on umount, and the `nano` wrapper's DOS line-ending conversion covers the diskimage mount point (`/tmp/ima_mount`) too. `.ima` files are raw superfloppy images with no partition table, so the mount is a plain `mount -o loop` with no partition offset — unlike VHDs. No legacy shortcut name (`mountima`) was added; the original toolkit never had floppy-image mounting, and the direction here is to avoid adding more shortcut names, not multiply them.

`create vhd`'s `[archive]` argument now also accepts a plain folder, matching `create diskimage`. `cmdVHDCreate`/`injectDOSArchive`/`createWin31VHD` (`vhdcreate.go`) previously only accepted `.zip`/`.tar`/`.tar.gz`/`.tar.bz2` via `isSupportedArchive`/`extractArchive`; a `copyDirTree` helper (`util.go`) now recursively copies a folder's contents the same way `extractArchive` unpacks an archive, and every call site branches on `isDir(source)` first. `promptDeleteSource` (`util.go`) offers to remove the original source after a successful injection, for either kind: an archive file via `os.Remove`, a folder via `os.RemoveAll`, same prompt and default (no) either way. It's shared by `create vhd -dos`/`-win31`'s archive injection and `create diskimage`, so the experience doesn't depend on source type or which command is used.

## Inherited from the original bash toolkit

Some of aotools' own lessons below are regressions of bugs the original bash `mkvhd`/`resizevhd`/`vhdmount` scripts already found and fixed.

**Zip attribute restoration misread Unix-made archives.** Reading a zip entry's `ExternalAttrs` byte as a DOS attribute hid files packaged by Unix-based zip tools, since only MS-DOS-origin entries store a real DOS attribute there. `zipAttrManifest` (`archext.go`) checks each entry's `CreatorVersion` host-OS byte and skips attribute restoration for anything not MS-DOS-origin.

**mtools handled all bulk file injection at first, to keep `mkvhd`/`mkima` root-free — the bulk-copy half was later rolled back.** mtools dispatches purely on the symlink name it's invoked as (`mcopy`, `mattrib`); `ensureMtoolsSymlinks()` (`util.go`) creates these automatically on first use. Its `-D o`/`-D O` flags (both needed, for long- vs. short-filename clashes) and its unrelated `-n` flag (suppressing a Unix-side "file exists" prompt on the mktemp placeholder used for reads) are still load-bearing in `mtoolsx.go` today. `mcopy -s` itself proved unreliable at real-world scale — raw FAT inspection on a ~70MB archive showed it silently dropping files and throwing false "No space left on device" errors — so every bulk *write* into a VHD now goes through `loopCopyIn` (kernel loop-mount + `cp -r`) instead, including `create diskimage`'s own archive injection. Small single-file writes, attribute restoration, and bulk *reads* (`mcopyOutTree`) remain on mtools, since they never showed the problem.

**`/tmp` is a small, RAM-backed filesystem, not general scratch space.** Staging a Windows 3.1 template plus archive extraction there exhausted its ~240MB tmpfs ceiling; `mktempBig()` stages on `/media/fat` instead (see bugs 8 and 10 below for the Go port's own instance of this).

**VHD/diskimage mount points live on tmpfs, not `/media/fat` or root.** MiSTer's root filesystem can get stuck read-only after an unclean shutdown; scratch and mount paths under `/tmp` (`vhdMountPoint`/`imaMountPoint`, `paths.go`) stay usable regardless of root's health. `chdMountPoint`/`chdExtractDir` are the deliberate exception, on `/media/fat`, since CD-sized CHD extraction can exceed tmpfs.

**The embedded MBR bootstrap (`bootstrap.go`'s `mbrBootstrapB64`) is real, LBA-based boot code, not the placeholder it replaced.** The original MBR boot code was an infinite loop, wrongly assumed to be overwritten by `FORMAT /S` (which only rewrites the partition's own boot sector, never the MBR). The current bootstrap relocates to `0000:0600`, finds the active partition, reads its boot sector via INT13h extended LBA (CHS fallback), verifies `0x55AA`, and chainloads it.

**MS-DOS 6.22 needs the plain FAT16 partition type (`0x06`), not the LBA variant (`0x0e`), plus a non-zero `TotalSec16` BPB field.** Without both, DOS never assigns drive C: a letter at all — `mkfs.vfat` zeroes `TotalSec16` in favor of `TotalSec32` for larger filesystems, and the original partitioning logic defaulted to the LBA type byte. `fixBPB` (`mbr.go`) always writes both correctly.

## Eleven bugs found and fixed during testing

1. **`vhd resize`: `COMMAND.COM` occasionally corrupted.** Writing `IO.SYS`/`MSDOS.SYS`/`COMMAND.COM` via one multi-file `mtools` batch sometimes left `COMMAND.COM` partly zeroed with a broken FAT chain. Fixed by routing those three files through the same kernel loop-mount + `cp` path used elsewhere instead of `mtools`.

2. **Archive injection: "invalid cross-device link."** Wrapper-folder flattening staged into `/tmp` while archive staging lives on `/media/fat` — different filesystems, which `os.Rename` can't cross. Fixed by staging as a sibling directory on the same filesystem.

3. **`install` idempotency broke under a renamed binary.** The check looked for a literal `"aotools shellinit"` string, so a differently-named test build duplicated the install block instead of being detected. Fixed by checking for the fixed comment-header line instead, independent of the binary's name.

4. **`promptLine` silently dropped the second of two sequential prompts.** A fresh `bufio.Reader` per call discarded buffered input under piped/scripted multi-line input, so every prompt after the first read back empty. Fixed by sharing one `bufio.Reader` across all `promptLine` calls.

5. **`install`'s PATH export never reached new SSH sessions.** `user-startup.sh` runs once at boot as a standalone process — anything it exports dies with that process and is never inherited by a later shell. Fixed by having it write `/etc/profile.d/aotools.sh` on every boot instead, which `/etc/profile` actually sources for login shells.

6. **`aotools mount vhd`/`umount vhd`, invoked directly, never changed directory like `mountvhd`/`umountvhd` did.** A subprocess can't change its parent shell's working directory, so mixing the two spellings corrupted the shared "previous directory" state. Fixed by making `aotools` itself a shell function that handles the cd for `mount`/`umount vhd`/`chd`, passing every other subcommand through to the binary.

7. **`resize vhd`: root directory entry order not preserved, breaking boot.** DOS 6.22 loads `IO.SYS`/`MSDOS.SYS` by root-directory *position*, not name, but the system-file copy list was built from `os.ReadDir`'s alphabetical order. Fixed by iterating the fixed `sysFiles` order (`IO.SYS`, `MSDOS.SYS`, `COMMAND.COM`) instead.

8. **`resize vhd`: boot sector and file content silently lost on large resizes.** Two separate `loopCopyIn` calls (system files, then everything else) meant the second call's loop-attach got a stale view of the root directory and overwrote the first call's entries. Fixed by merging both into one `loopCopyIn` call and moving the boot sector merge to run last, after that single copy.

9. **`create vhd -win31`: the same two bugs as above, in `createWin31VHD`.** It cloned its template through the same alphabetical-order, two-call pattern as `resize vhd`. Fixed identically: fixed system-file order, one consolidated `loopCopyIn` call, boot sector merge last.

10. **`resize vhd` and `create diskimage`'s staging directories defaulted to `/tmp`.** Both used plain `os.MkdirTemp("", ...)` instead of the project's `mktempBig()`, so staging a Windows 3.1 VHD's content overflowed `/tmp`'s tmpfs. Fixed by switching both to `mktempBig()`, which stages on `/media/fat`.

11. **`fatal()`'s `os.Exit(1)` skipped deferred cleanup, leaking staging directories and half-built replacement VHDs on failure.** `os.Exit` terminates immediately without running any `defer`. Fixed by having `fatal()` panic instead, recovered in `main()`'s `run()` wrapper — a genuine unrelated panic is still re-panicked and crashes visibly rather than being swallowed.

## Performance

`mount chd`'s raw-sector stripping step (`stripRawSectors`, used for MODE1/2352 and MODE2/2352 CD images) read/wrote one 2352-byte sector at a time with unbuffered `os.File` calls — roughly 350,000 syscall pairs per typical CD image, measuring out to ~1MB/s on ARM. Wrapping both sides in 1MB `bufio` buffers brought it up to tens of MB/s.

## Screenshot-based boot verification and Windows 3.1 / graphical modes

The MiSTer's native screenshot mechanism produces a consistent, garbled, striped pattern for every Windows 3.1 boot — reproduced on both freshly-created VHDs and pre-existing, already-working Windows 3.1 content, so this is a limitation of that capture path for this video mode, not a defect in any `aotools`-created VHD. The pattern changes in response to keyboard input, confirming the core is alive and rendering rather than hung. Boot correctness in these cases is instead confirmed at the disk level (root directory order, boot sector bytes, MBR/BPB fields) plus confirmation that plain MS-DOS boots cleanly before Windows 3.1 takes over. An OBS-based capture (reading actual video output rather than decoding the core's internal framebuffer) is a more reliable alternative for this class of verification going forward.
