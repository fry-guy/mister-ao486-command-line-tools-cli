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

The build's worst bug: `createcd`/`createraw` crashed with SIGBUS whenever the LZMA codec (`cdlz`, chdman's own default) was used. It initially looked like an ARM alignment fault in LZMA's match-finder code, but the real cause was a stale object file compiled under different NEON codegen flags that `make`'s incremental build never recompiled once those flags changed elsewhere in the build — a fully clean rebuild removed the crash entirely.

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

`create vhd`'s `[archive]` argument now also accepts a plain folder, matching `create diskimage`. `cmdVHDCreate`/`injectDOSArchive`/`createWin31VHD` (`vhdcreate.go`) previously only accepted `.zip`/`.tar`/`.tar.gz`/`.tar.bz2` via `isSupportedArchive`/`extractArchive`; a `copyDirTree` helper (`util.go`) now recursively copies a folder's contents the same way `extractArchive` unpacks an archive, and every call site branches on `isDir(source)` first. The "delete the original source?" prompt after injection is skipped entirely for folder sources — offering to `rm -rf` a folder the caller pointed at (potentially their own working directory) is a materially different risk than offering to delete a redundant zip, so it's simply not offered rather than defaulting to "no." Verified end-to-end: a `create vhd -dos` run against a folder source completed the full QEMU-driven install, copied the folder into `\<GAMENAME>` via `copyDirTree`, updated `AUTOEXEC.BAT`, and left the source folder untouched.

## Inherited from the original bash toolkit

Some of aotools' own hard-won lessons below aren't new discoveries — they're regressions of problems the original bash `mkvhd`/`resizevhd`/`vhdmount` scripts had already found and fixed, before the Go port existed.

**mtools was integrated into the original bash toolkit specifically to remove `mkvhd`/`mkima`'s dependency on root access for file-injection steps — but the bulk-copy half of that integration was later rolled back.** mtools has no "pass the subcommand as an argument" mode; it dispatches purely on the name of the symlink used to invoke it (`mcopy`, `mattrib`), which is why `ensureMtoolsSymlinks()` (`util.go`) recreates those symlinks automatically the first time they're needed rather than requiring a manual setup step. Two real argument-semantics bugs surfaced during that integration, both still load-bearing in `mtoolsx.go`'s helpers today: `mcopy`'s `-D o`/`-D O` flags are both required (lowercase handles long-filename clashes, uppercase handles short-filename clashes — using only one silently leaves the other case unprotected), and a completely separate Unix-side "destination file already exists" prompt (triggered by copying into an already-created `mktemp` placeholder file) needed mtools' unrelated `-n` flag to suppress. Both mattered because an unhandled interactive prompt with no attached terminal just hangs silently.

Direct raw FAT byte-level inspection on a real ~70MB, 140+ file archive later proved `mcopy -s` (the recursive bulk-copy mode) silently drops files — sometimes entire subdirectories — partway through a large copy, and separately throws false "No space left on device" errors with well over 90MB genuinely free. This didn't undo the root-freedom win everywhere: `create diskimage` stayed entirely root-free (its archives are capped at 2.88MB, far below the size that exposed the bug), and every small, single-file operation (`AUTOEXEC.BAT`/`CONFIG.SYS`/`WIN.INI` writes, attribute restoration via `mattrib`) plus bulk *reads* out of an existing VHD (`mcopyOutTree`, used by `resize vhd`'s initial read/verification and `create vhd -win31`'s template read) never showed the problem and stayed on mtools. What changed: every bulk *write* into a VHD — `resize vhd`'s system-file copy, `create vhd -win31`'s template and archive/game-injection copies, and `create vhd -dos`'s archive injection — was converted to a `loopCopyIn` helper (kernel loop-mount plus `cp -r`) instead, trading back the root-freedom those specific paths had gained for correctness at real-world scale. This is the same failure mode as bug 1 below; the Go port's initial system-files copy in `resize vhd` apparently never received this conversion in the original bash script (it was flagged as a known follow-up item that never got done there), so testing surfaced the identical corruption independently once the Go port was built.

**Update: the `create diskimage` asymmetry above has since been closed.** `cmdIMACreate` (`ima.go`) now copies its archive/folder source in via `loopCopyIn` as well, the same as every VHD-side path — `mcopyPutRecursive` is no longer called anywhere in the codebase. The conversion was verified against both source types using real Deathbringer floppy content: a 6-file, ~1.6MB zip archive (`DISK1.zip`, wrapper-folder-flattened first) and a 24-file, ~3.2MB plain folder (`disk2`), each written to a floppy image and confirmed byte-for-byte identical to the source via a real kernel `mount -t vfat` read-back, not just mtools reading back its own writes. Floppy sizes never came close to exposing the original `mcopy -s` bug (the archive that did was ~70MB against a 2.88MB ceiling here), so this was a preventive fix for consistency rather than a response to an observed failure.

**`/tmp` is a small, RAM-backed filesystem, not a general scratch area.** The bash toolkit already discovered `/tmp`'s ~240MB tmpfs ceiling was too small for staging a Windows 3.1 template plus archive extraction, and introduced a `mktemp_big()` helper to stage on `/media/fat` instead. This is the same lesson as bugs 8 and 10 below.

**The mount points this toolkit uses for VHDs and disk images (`/tmp/vhd_mount`, `/tmp/ima_mount`) are deliberately on tmpfs, not on `/media/fat` or root.** This traces back to a real incident where MiSTer's root filesystem — normally mounted read-write — ended up stuck read-only after an unclean shutdown, confirmed via `dmesg` showing a journal-recovery-required boot sequence that repeated on every subsequent reboot without ever resolving to read-write. The original `mkvhd -dos`/`vhdmount` both depended on scratch mountpoints under `/mnt`, on the root filesystem, and broke the moment a new one needed creating there. The fix — relocating every such scratch/mount path onto `/tmp` (tmpfs, always writable regardless of root's state) — means none of these tools depend on root filesystem health at all. That's why aotools' own `vhdMountPoint`/`imaMountPoint` (`paths.go`) sit on `/tmp` while `chdMountPoint`/`chdExtractDir` deliberately sit on `/media/fat` instead: CHD extraction can involve full CD/DVD-sized images that would blow through tmpfs, so it accepts the size risk in exchange for avoiding the root-dependency risk.

**The embedded MBR bootstrap code (`bootstrap.go`'s `mbrBootstrapB64`) replaced an original placeholder that was never real bootstrap code.** The first version of `make_mbr.py` wrote `CLI; XOR AX,AX; MOV SS,AX; MOV SP,7C00h; STI; JMP $` — an infinite loop — into the MBR's boot code area, on the mistaken assumption that `FORMAT /S` would overwrite it later. It doesn't: `FORMAT /S` rewrites the partition's own boot sector at LBA 128, never the MBR at LBA 0. This was invisible through the DOS-install pipeline's own automated testing, which only ever booted from the floppy to run FORMAT/XCOPY and never rebooted to let the BIOS chainload from the hard disk itself, so it only surfaced on a real hardware boot, which hung forever right after "Booting from Hard Disk...". The fix was a genuine, standard MBR bootstrap hand-assembled in NASM: relocate to `0000:0600`, scan the partition table for the active (`0x80`) entry, load its boot sector via INT13h extended LBA read (falling back to CHS for BIOSes without extensions), verify the `0x55AA` signature, then chainload it. The first attempt used CHS-only addressing and hit a BIOS geometry-translation mismatch; switching to LBA addressing fixed it cleanly, tested first in a local x86 QEMU sandbox before ever touching the MiSTer. This assembled bootstrap is exactly what `mbrBootstrapB64` embeds today.

**MS-DOS 6.22 silently refuses to assign a drive letter to a FAT16-LBA (`0x0e`) partition — it only recognizes the plain FAT16 type (`0x06`).** Found the hard way: a freshly created, correctly formatted VHD would boot, but DOS never saw drive C: at all ("Invalid drive specification"), even though SeaBIOS itself detected the disk fine. The partition table entry's type byte was `0x0e` (the LBA-addressed FAT16 type the original partitioning logic wrote by default); changing it to `0x06` made C: appear immediately. The same investigation found `TotalSec16` (a 16-bit BPB field) also needs to be non-zero — `mkfs.vfat` zeroes it and relies on the 32-bit `TotalSec32` field instead for larger filesystems, which MS-DOS 6.22 can't read. Both fixes are exactly what `fixBPB` (`mbr.go`) does today: it always writes a real `TotalSec16` (falling back to `TotalSec32` only when the sector count doesn't fit in 16 bits) and always resolves the MBR partition-type byte to `0x01`/`0x04`/`0x06`, never the LBA variant. A second, subtler instance of the same partition-type bug turned up in `bootrecord.vhd` itself (the DOS/MISTER/DRIVERS/UTIL source used during `-dos` installs) — it carried the identical `0x0e` byte, which is why that reference drive wasn't getting a letter either even after the target VHD's own partition table had already been fixed.

## Eleven bugs found and fixed during testing

1. **`vhd resize`: corrupted `COMMAND.COM` on some runs.** Writing `IO.SYS`/`MSDOS.SYS`/`COMMAND.COM` via a single multi-file `mtools` batch (matching the original bash script) occasionally produced a partly-zeroed `COMMAND.COM` with a broken FAT chain, even though the identical batch succeeded on isolated retries. Fixed by routing those three files through the same kernel loop-mount + `cp` path used everywhere else in the codebase instead of raw `mtools`.

2. **Archive injection: "invalid cross-device link."** The wrapper-folder-flattening step staged into `/tmp` while the archive staging area lives on `/media/fat` — a different filesystem, which `os.Rename` can't cross (unlike the `mv` command, which falls back to copy+delete transparently). Fixed by staging as a sibling directory on the same filesystem.

3. **`install` idempotency broke under a renamed binary.** The idempotency check looked for a literal `"aotools shellinit"` substring in `user-startup.sh`, assuming the binary is always invoked as `aotools`. A differently-named test build (`aotools_new`) wrote `.../aotools_new shellinit` instead, the check silently missed it, and a duplicate block got appended. Fixed by checking for the fixed comment-header line instead, which doesn't depend on the binary's filename. Running `install` twice now appends exactly once, regardless of binary name.

4. **`promptLine` silently dropped the second of two sequential prompts.** `promptLine` created a new `bufio.Reader` on stdin per call. With a real interactive terminal this is invisible (input arrives one line at a time), but with piped/scripted input spanning multiple lines, the first call's reader buffers ahead and is discarded along with everything after the first line — every prompt after the first comes back empty. Surfaced by the install-time download flow's two sequential prompts under piped test input: the second answer always read as blank, cancelling the download. Fixed by sharing one `bufio.Reader` across every `promptLine` call.

5. **`install`'s PATH export never reached new SSH sessions.** The first version had `install` write `export PATH=...` / `eval "$(... shellinit)"` directly into `user-startup.sh`. Tested via a `bash -c` subshell of an already-live SSH session, which passed — but that subshell inherits its parent's already-exported environment, so the test proved nothing about a fresh login. After a real reboot and a genuinely new SSH session, `aotools` still wasn't found. Root cause: `user-startup.sh` runs once as a standalone child process at boot; anything it exports dies with that process and is never inherited by any shell started afterward, reboot or not. Fixed by having `install` stop exporting/evaling inside `user-startup.sh` directly — instead `user-startup.sh` (re)writes `/etc/profile.d/aotools.sh` on every boot, since `/etc/profile` (which login shells do source) sources every `*.sh` file under `/etc/profile.d/`. `install` also writes that file immediately, so the fix takes effect without a reboot. Re-verified via `bash -lc '...'` (a genuine login shell) that PATH and the shell functions are both present in a fresh session.

6. **`aotools mount vhd` / `aotools umount vhd`, invoked directly, never changed directory — unlike `mountvhd`/`umountvhd`.** Two spellings of the same operation behaved differently under one name, and mixing them corrupted the shared "previous directory" state: mounting via the raw binary then unmounting via the shell function landed in `/tmp` instead of the original directory, since the raw path never set `AOTOOLS_VHD_PREV_DIR`/`AOTOOLS_CHD_PREV_DIR`. Root cause: `aotools mount vhd` run directly executes the compiled binary as a subprocess, which can never change its parent shell's working directory — only a shell function can. Fixed by making `aotools` itself a shell function (`shellFunctionsTemplate`, `shellinit.go`) that special-cases `mount vhd`/`umount vhd`/`mount chd`/`umount chd` to handle the cd and track the previous directory, passing every other subcommand straight through to the binary. `mountvhd`/`umountvhd`/`mountchd`/`umountchd`/`mkvhd`/etc. are now thin wrappers around this same function, so there's a single code path and a single set of state variables — no way to reach an inconsistent state by mixing spellings. Verified against both a VHD and a CHD, in every combination of the two spellings on mount and umount.

7. **`resize vhd`: root directory entry order not preserved, breaking boot.** A resized VHD passed the tool's own byte-for-byte content verification but failed to boot with "Non-System disk or disk error," reproduced across independent, isolated runs. Root cause: MS-DOS 6.22's boot sector loads `IO.SYS`/`MSDOS.SYS` by root-directory-entry *position*, not by name — `IO.SYS` must be the first root directory entry, `MSDOS.SYS` second, or the disk won't boot even with perfectly valid file content and boot code. `cmdVHDResize` (`vhdresize.go`) already special-cased copying these three files first, but built the list by iterating `os.ReadDir`'s alphabetically-sorted entries and checking membership — which silently produced alphabetical order (`COMMAND.COM`, `IO.SYS`, `MSDOS.SYS`) instead of canonical boot order, confirmed via a raw kernel loop-mount listing showing `COMMAND.COM` as entry 1. Fixed by iterating the fixed `sysFiles` order (`IO.SYS`, `MSDOS.SYS`, `COMMAND.COM`) and searching the directory entries for each by name, rather than the reverse. Re-verified across three resize sizes (40MB, 100MB, 2047MB), confirming `IO.SYS` lands as entry 1 every time.

8. **`resize vhd`: boot sector and file content silently lost on large (near-2047MB) resizes.** Found immediately after fixing bug 7, while boot-testing the largest resize size. The BIOS reported "This is not a bootable disk," and the final boot sector still carried `mkfs.vfat`'s own defaults (OEM string `mkfs.fat`, 255 heads) instead of the merged real-DOS values, even though both `fixBPB` and `writeBootSectorMerge` had logged success. Moving the boot sector write to run after all `loopCopyIn` calls surfaced a second, more serious symptom: `IO.SYS`/`MSDOS.SYS`/`COMMAND.COM` were missing entirely from the final file, confirmed via a raw kernel loop-mount listing rather than an mtools quirk. The original code made two independent `loopCopyIn` calls — one for the three system files, one for everything else — each its own loop-attach/mount/copy/unmount/detach cycle. The pattern (first call's writes lost, second call's intact) is consistent with the second call's fresh loop-attach getting a stale, pre-first-call view of the root directory on this kernel/loop-driver combination, and overwriting the first call's entries as if their slots were still free. Fixed by merging both copies into one `loopCopyIn` call — one combined, order-preserving list, one mount session — and moving the boot sector merge to run last, after that single copy and before the mtools-only (non-loop) attribute application and verification steps. Re-verified at 2047MB: all three system files present in correct order, boot sector shows the correct merged `MSDOS5.0` OEM string and 16 heads, verification genuinely passes, and the VHD boots to the game splash screen.

9. **`create vhd -win31`: the same root-cause bugs as 7 and 8, in `createWin31VHD`.** `createWin31VHD` (`vhdcreate.go`) clones its template through the same format-then-copy-then-merge-boot-sector pattern as `resize vhd`, rather than a QEMU-driven install — which is why fresh `-dos` VHDs were never affected (QEMU's own FORMAT/SYS writes system files directly). It built its system-files list by iterating alphabetically-sorted entries (bug 7's pattern), used separate `loopCopyIn` calls for main content and archive/game injection (bug 8's pattern), and ran `writeBootSectorMerge` before all of them. Fixed identically: system-files list built from the fixed `sysFiles` order; main content copy consolidated into one `loopCopyIn` call; boot sector merge moved to run last, after the main content copy and any archive/game injection copy, with nothing loop-based touching the file afterward. Verified by cloning a real Windows 3.1 game via `create vhd -win31` with a folder source: correct system-file order and a correctly-merged boot sector (`MSDOS5.0` OEM, matching BPB) confirmed by direct inspection, and DOS boots cleanly before Windows 3.1 itself takes over.

10. **`resize vhd` and `create diskimage`'s archive-staging path: staging directories defaulted to `/tmp`, a small tmpfs.** Found while resizing a Windows 3.1 VHD (larger content than the plain-DOS VHDs bugs 7-9 were tested against) — the resize failed with "No space left on device" during its own verification step. `/tmp` on this device is a ~240MB tmpfs; `cmdVHDResize`'s `stageDir`/`verifyDir` (`vhdresize.go`) and `cmdIMACreate`'s archive-staging directory (`ima.go`) all used plain `os.MkdirTemp("", ...)`, which defaults to `/tmp`, instead of the project's own `mktempBig()` helper — already used correctly by `createWin31VHD`/`injectDOSArchive` for the same reason: staging real file content needs real disk space, not RAM-backed tmpfs. Fixed by switching all three spots to `mktempBig()`, which stages under a scratch directory on `/media/fat`. (`loopfs.go`'s `loopCopyIn` mount point and `vhdcreate.go`'s `dosOverheadBytes` mount point were left alone — both are empty mount points for a loop-mounted filesystem, not bulk-copy targets, and carry none of this risk.) Re-verified: the same Windows 3.1 resize now completes and verifies successfully, with `/tmp` usage staying under 10MB instead of climbing into the hundreds of megabytes.

11. **`fatal()`'s `os.Exit(1)` skipped deferred cleanup, leaking staging directories (and, on `resize vhd`, the half-built replacement container) on any failure partway through.** This is what bug 10 above was actually running into — `/tmp` filled up specifically because of orphaned `stageDir`/`verifyDir` directories left behind by earlier failed runs. Root cause: `os.Exit()` terminates the process immediately and skips every registered `defer` on the call stack, so `defer os.RemoveAll(stageDir)` (and, in `resize vhd`, `defer os.Remove(newVHD)` for the half-built replacement container) never ran when a downstream `fatal()` fired. Fixed by changing `fatal()` (`util.go`) to `panic(fatalExit{})` instead of calling `os.Exit(1)` directly — a panic unwinds the stack and runs every `defer` along the way before anything else happens, which is the cleanup guarantee `os.Exit` doesn't provide. `main()` (`main.go`) now defers to a `run()` function holding the original dispatch logic, wrapped in a `recover()`: a recovered `fatalExit` exits with status 1 (matching prior behavior, after cleanup has run); any other panic value is re-panicked, so a genuine bug still crashes visibly with a full stack trace instead of being mistaken for a clean error exit. Verified by forcing `fatal()` after staging directories were already populated, for all three affected commands (`resize vhd`, `create vhd -win31`, `create diskimage`) — same error message and exit code as before, but the scratch directory is now actually gone afterward. Also confirmed with a standalone test program that a genuine, unrelated panic (an out-of-range index) is still re-panicked and crashes with its own stack trace and a non-1 exit code, rather than being swallowed.

## Performance

`mount chd`'s raw-sector stripping step (`stripRawSectors`, used for MODE1/2352 and MODE2/2352 CD images) read/wrote one 2352-byte sector at a time with unbuffered `os.File` calls — roughly 350,000 syscall pairs per typical CD image, measuring out to ~1MB/s on ARM. Wrapping both sides in 1MB `bufio` buffers brought it up to tens of MB/s.

## Screenshot-based boot verification and Windows 3.1 / graphical modes

The MiSTer's native screenshot mechanism produces a consistent, garbled, striped pattern for every Windows 3.1 boot — reproduced on both freshly-created VHDs and pre-existing, already-working Windows 3.1 content, so this is a limitation of that capture path for this video mode, not a defect in any `aotools`-created VHD. The pattern changes in response to keyboard input, confirming the core is alive and rendering rather than hung. Boot correctness in these cases is instead confirmed at the disk level (root directory order, boot sector bytes, MBR/BPB fields) plus confirmation that plain MS-DOS boots cleanly before Windows 3.1 takes over. An OBS-based capture (reading actual video output rather than decoding the core's internal framebuffer) is a more reliable alternative for this class of verification going forward.
