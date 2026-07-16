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
