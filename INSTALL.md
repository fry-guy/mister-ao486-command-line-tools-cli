# aotools

**For step-by-step install instructions and full command syntax written for end users, see `README.md`. This file is the build/development notes: what was tested, bugs found and fixed, and how to rebuild from source.**

A single-binary Go port of the ao486 DOS Toolkit (`mkvhd`, `resizevhd`, `mkmgl`, `mkchd`, `mkima`, `mountvhd`/`umountvhd`, `mountchd`/`umountchd`, plus the `make_mbr.py`/`fix_bpb.py`/`newgame_controller.py` helpers they drove). Ported from your real, running scripts on `/media/fat/linux` — nothing here was guessed from the README alone.

**Already deployed and smoke-tested on your MiSTer** at `/media/fat/linux/aotools/aotools` — none of the original files were touched; everything lives in that new folder. You can start using it right now over SSH.

**Everything is one file.** The `mountvhd`/`umountvhd`/`mountchd`/`umountchd`/`mkvhd`/`resizevhd`/`mkmgl`/`mkchd`/`mkima` shell functions used to live in a separate `aotools-functions.sh` you had to remember to source. They're now embedded directly inside the `aotools` binary itself and are wired up automatically by a one-time `aotools install` — there is no second file to install or track.

## What's here

- `*.go`, `go.mod` — full source (single `package main`, no external dependencies beyond the Go standard library)
- This file

## One-time setup on your MiSTer

```
/media/fat/linux/aotools/aotools install
```

This appends a small marker-delimited block to `/media/fat/linux/user-startup.sh` (MiSTer's own persistent boot hook):

```
# --- aotools:begin (shell functions + PATH) ---
cat > /etc/profile.d/aotools.sh <<'AOTOOLS_PROFILE_EOF'
export PATH="$PATH:/media/fat/linux/aotools"
eval "$(/media/fat/linux/aotools/aotools shellinit)"
AOTOOLS_PROFILE_EOF
# --- aotools:end ---
```

Why it's shaped this way (this took a real bug to get right — see item 5 below): `user-startup.sh` runs once, as its own standalone child process at boot, so anything it `export`s or `eval`s directly dies with that one process and never reaches an SSH session opened afterward. What *does* reach every new login shell is `/etc/profile`, which on MiSTer already sources every `*.sh` file under `/etc/profile.d/`. So instead of exporting anything itself, the block above has `user-startup.sh` (re)write `/etc/profile.d/aotools.sh` — containing the real `export PATH=...` / `eval "$(... shellinit)"` lines — on every single boot. `install` also writes that same file immediately when you run it, so it works right away, without waiting for a reboot.

It's idempotent (checks for the begin marker and the current mechanism first, so running `install` again is a no-op) and only ever appends to `user-startup.sh` — it never rewrites or touches anything else already there. From the moment you run `install` onward, any *new* SSH session (no reboot required — though it also survives reboots) has `aotools` on `$PATH` (so `aotools <command>` works from any directory) and `mountvhd`/`umountvhd`/`mountchd`/`umountchd`/`mkvhd`/`resizevhd`/`mkmgl`/`mkchd`/`mkima` just available, no period, no sourcing, nothing else to remember.

If a machine already has an older install — either the very first format (shell functions only, no PATH) or the version right after that (which tried to export/eval directly in `user-startup.sh` and silently didn't work for new SSH sessions) — running `install` again detects that automatically, strips the old block, and replaces it with the current one above.

To get both in your *current* shell right now without opening a new session:

```
export PATH="$PATH:/media/fat/linux/aotools"
eval "$(/media/fat/linux/aotools/aotools shellinit)"
```

## Using it on your MiSTer right now

```
/media/fat/linux/aotools/aotools vhd create -dos mygame.vhd game.zip
/media/fat/linux/aotools/aotools mgl create -dos "My Game" .
```

## Full command reference

```
aotools vhd create [-dos|-win31] <name.vhd> [archive]
aotools vhd resize <name.vhd>
aotools mgl create -dos|-win31 "Display Name" [source_folder]
aotools chd create <input.iso|.cue|.bin|.gdi> <output.chd>
aotools ima create <name.ima> [source] [-s size]
aotools mount vhd <name.vhd>      (or `mountvhd <name.vhd>` via the shell function)
aotools umount vhd                (or `umountvhd`)
aotools mount chd <name.chd>      (or `mountchd <name.chd>`)
aotools umount chd                (or `umountchd`)
aotools install                   (one-time: wire shell functions + PATH into every future shell)
aotools uninstall                 (reverses install -- removes the user-startup.sh wiring only)
aotools shellinit                 (prints the shell functions; used internally by `install`)
aotools doctor                    (reports on qemu/chdman/templates/mtools/etc. -- see README.md)
```

## Rebuilding it yourself

No compiler needed to *use* it — the binary at `/media/fat/linux/aotools/aotools` is a static ARMv7 executable, drop-in runnable. You'd only rebuild if you want to modify the source.

On the MiSTer itself (it has no compiler by default, but does have internet access):

```
curl -sSkL -o /tmp/go.tar.gz https://go.dev/dl/go1.26.5.linux-armv6l.tar.gz
mkdir -p /media/fat/linux/.aotools_buildtools/goroot
tar -C /media/fat/linux/.aotools_buildtools/goroot -xzf /tmp/go.tar.gz --strip-components=1
export GOROOT=/media/fat/linux/.aotools_buildtools/goroot PATH=$GOROOT/bin:$PATH
export GOCACHE=/media/fat/linux/.aotools_build_cache GOPATH=/media/fat/linux/.aotools_gopath
cd /path/to/this/source
go build -ldflags="-s -w" -o aotools .
```

First build compiles the whole standard library (a few minutes on ARMv7); rebuilds after that are seconds. **Extract the Go toolchain and set `GOCACHE`/`GOPATH` under `/media/fat`, not `/tmp`** — `/tmp` is a ~240MB tmpfs and is nowhere near big enough for a full Go toolchain + build cache.

Or cross-compile from any machine with Go installed:
```
GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 go build -ldflags="-s -w" -o aotools .
```

## What was tested on real hardware

All of this ran end-to-end on your actual MiSTer during development, not just compiled:

- `vhd create` blank, `-dos` (full QEMU-driven FORMAT+XCOPY install), `-win31` (template clone + boot sector preservation), and `-dos` with a zip archive injected (wrapper-folder flattening, launch-executable picker, AUTOEXEC.BAT update)
- `vhd resize` (4MB → 8MB, verified byte-for-byte against the original, 3 runs)
- `mgl create`
- `ima create` blank, sized, and with a source directory injected
- `mount vhd` / `umount vhd`
- `mount chd` / `umount chd`, validated end-to-end against a real Simon the Sorcerer II CD image (`.cue`/`.bin` → mounted, genuine game files visible)
- `install` / `shellinit`: idempotency (running `install` twice appends the wiring line only once), and the full `eval "$(aotools shellinit)" → mountvhd game.vhd → cd's into it → umountvhd → cd's back out` flow against a real DOS-formatted VHD -- confirmed the shell actually changes directory with no leading `.` needed at call time (only `install`/`eval` need it, once)
- `doctor`: confirmed it correctly reports `[ok]` for every dependency on a MiSTer that already has the original ao486 DOS Toolkit set up
- A full, real `chd create` end-to-end run against the user's own Simon the Sorcerer II CD (`SIMON2.CUE`/`SIMON2.BIN`, ~407MB raw) -- produced a valid 177MB `.chd` (`chdman info` confirms correct track metadata), which was then itself successfully mounted via `aotools mount chd` and showed the genuine game installer files, proving the full create → mount round trip works on freshly-created output, not just on a CHD someone else made
- `install`'s dependency-download offer: tested by safely renaming real dependencies (`mtools`, then separately `dos_template.vhd`) aside (not deleting them), then running the full flow against real user-startup.sh/network conditions -- declined first (confirmed it skips cleanly with no side effects), then accepted with the "I agree" acknowledgment (confirmed each download completes, lands at the exact right path with the right permissions, and is byte-for-byte identical to the original file), then restored the originals and cleaned up all test artifacts. The `dos_template.vhd` link was corrected mid-project (the first version pointed at the same archive.org file as `win31_template.vhd`, almost certainly a paste error) -- re-verified against the corrected link.
- `uninstall`: tested against the real, live `user-startup.sh` (backed up first) -- confirmed it cleanly removes exactly the block `install` added and nothing else (rest of the file byte-for-byte unchanged), confirmed `install` cleanly re-adds it afterward, and confirmed running `uninstall` a second time (nothing installed) correctly reports "Not installed" instead of erroring or corrupting the file. Real file restored from backup and re-verified `install`-idempotent afterward before moving on.
- `install` adding `aotools` itself to `$PATH` (and `uninstall` removing it) -- first pass (later found broken): backed up the real, live `user-startup.sh`, then upgraded it to a block that ran `export PATH=...` / `eval "$(...)" ` directly inside `user-startup.sh`. Verified via a `bash -c` *subshell of the already-live SSH session* that the functions/PATH worked -- which, in hindsight, was the wrong test: that subshell inherited its parent's already-exported environment rather than genuinely proving a *fresh* SSH login would see it.
- **Real-world regression caught by the user**: after actually opening a new SSH session (and even after a full reboot), `aotools <command>` still reported "command not found". Investigated directly on the device: `user-startup.sh` is executed once, as a standalone child process, by `/etc/init.d/S99user` at boot -- any `export`/`eval` inside it is scoped to that one process and is never inherited by SSH sessions started afterward, no matter how many reboots happen. Confirmed via `bash -lc 'echo $PATH'` in a genuine login shell that the directory was simply never added. Also confirmed `/etc/profile` (which MiSTer's login shells do source) hard-resets `PATH` to a fixed string and separately loops over and sources every `*.sh` file in `/etc/profile.d/` -- the real hook that reaches every login shell.
- **Fix**: `install` no longer tries to export/eval inside `user-startup.sh` at all. Instead it has `user-startup.sh` (re)write `/etc/profile.d/aotools.sh` (containing the real `export PATH=...` / `eval "$(...)" ` lines) on every boot, and `install` also writes that file immediately for instant effect. Verified the whole flow end-to-end against the real device: backed up the real `user-startup.sh`; ran dry-run tests with a `/tmp`-copied test binary (legacy-format upgrade, old-broken-current-format upgrade, install-twice idempotency, uninstall, uninstall-twice) confirming `user-startup.sh` and `/etc/profile.d/aotools.sh` end up in exactly the expected state each time, then restored from backup; deployed the real fixed binary and ran the real, permanent `aotools install`; and finally verified with `bash -lc '...'` (a genuine login shell, matching what an actual interactive SSH terminal session gets) that `which aotools` resolves, `aotools -v` works, and -- the exact command that failed for the user -- `cd /media/fat/games/ao486/media/simon2 && aotools mount vhd simon2.vhd` succeeds. Cleaned up by unmounting the test VHD and deleting the temporary backup file.
- **`aotools mount/umount` unified with the shell-function shortcuts** (see bug 6 above): after fixing PATH, the user pointed out `aotools mount vhd`/`aotools umount vhd` (typed directly) still didn't `cd`, unlike `mountvhd`/`umountvhd` -- an inconsistency, not the "one command that works everywhere" that was asked for. Fixed by making `aotools` itself a shell function that handles the cd for `mount vhd`/`umount vhd`/`mount chd`/`umount chd` and passes everything else straight through to the binary; `mountvhd`/`umountvhd`/`mountchd`/`umountchd`/`mkvhd`/etc. now just call this same function. Verified on the real device against a real VHD (`simon2.vhd`) and a real CHD (`ultima8.chd`, chosen over the much larger `simon2.chd` purely for faster extraction during testing): `aotools mount/umount vhd`, legacy `mountvhd`/`umountvhd`, and mixed mount-via-one-form/umount-via-the-other all correctly `cd`'d in on mount and back to the exact starting directory on umount -- then the same four combinations again for `chd`, same result.

`chd create` was exercised end-to-end against that same real CD image, producing a fresh `.chd` that was itself then successfully mounted (see below).

## Four real bugs found and fixed during testing (worth knowing about)

1. **`vhd resize`**: writing `IO.SYS`/`MSDOS.SYS`/`COMMAND.COM` via a single multi-file `mtools` batch (matching the original bash script's own approach) came back with a corrupted `COMMAND.COM` on one real run — partly zeroed, wrong FAT chain — even though the identical batch succeeded in isolated retries. Fixed by routing those three files through the same reliable kernel loop-mount + `cp` path used everywhere else in the codebase, instead of raw `mtools`.
2. **Archive injection**: the wrapper-folder-flattening step failed with "invalid cross-device link" because its staging directory defaulted to `/tmp` while the archive staging area lives on `/media/fat` — a different filesystem, which `os.Rename` (unlike the `mv` *command*, which silently falls back to copy+delete) can't cross. Fixed by staging as a sibling directory instead.
3. **`aotools install` idempotency**: the first version checked for a literal `"aotools shellinit"` substring in `user-startup.sh` to decide whether it was already installed, assuming the binary would always be invoked as `aotools`. Testing with a differently-named test build (`aotools_new`) showed the check silently failed and re-appended a duplicate block, since the actual written line was `.../aotools_new shellinit`, not `.../aotools shellinit`. Fixed by checking for the fixed comment-header line instead, which doesn't depend on the binary's own filename. Re-verified: running `install` twice now only ever appends once.
4. **`promptLine` losing input across sequential prompts**: `promptLine` (used everywhere aotools asks the user a question) created a brand-new `bufio.Reader` on stdin every single call. That's invisible with a real interactive terminal (input arrives one line at a time), but breaks with piped/scripted input spanning multiple lines: the first call's `bufio.Reader` buffers ahead and silently swallows everything after the first line, then gets thrown away, so every prompt after the first one comes back empty. Caught directly while testing the new install-time download flow (which asks two questions in a row -- "download?" then the "I agree" acknowledgment) with piped test input: the second answer kept coming back as if the user had pressed Enter with nothing typed, incorrectly cancelling the download every time. Fixed by sharing one `bufio.Reader` across every `promptLine` call instead of creating a new one each time. Re-verified with the same two-prompt flow: both answers are now read correctly.
5. **`install`'s PATH export never actually reached new SSH sessions**: the first version of the PATH feature had `install` write `export PATH=...` / `eval "$(... shellinit)"` directly into `user-startup.sh`. This looked correct when tested via a `bash -c` subshell spawned from within the same already-live SSH session (it inherited the parent shell's already-exported PATH), but that test was flawed -- it didn't prove anything about a genuinely new login. On the real device, after a real reboot and a real new SSH session, `aotools <command>` still reported "command not found." Root cause: `user-startup.sh` is run once, as a standalone child process, by `/etc/init.d/S99user` at boot; anything it exports dies with that one process and is never inherited by any shell started afterward, reboot or not. Fixed by having `install` stop exporting/evaling inside `user-startup.sh` entirely -- instead, `user-startup.sh` now (re)writes `/etc/profile.d/aotools.sh` on every boot (containing the real export/eval lines), since `/etc/profile` -- which MiSTer's login shells genuinely do source -- loops over and sources every `*.sh` file in `/etc/profile.d/`. `install` also writes that file immediately for instant effect, no reboot needed. Re-verified via `bash -lc '...'` (a genuine login shell, matching a real interactive SSH terminal) that PATH and the shell functions are both present in a fresh session, and that the user's exact originally-failing command now succeeds.
6. **`aotools mount vhd`/`aotools umount vhd` (typed directly) never actually `cd`'d anywhere, unlike `mountvhd`/`umountvhd`**: flagged by the user as unacceptable -- the whole point of a unified `aotools` command was for it to behave the same way everywhere, and having `aotools mount vhd x.vhd` silently *not* `cd` you in (while `mountvhd x.vhd` did) was two different behaviors under one name. Worse, mixing the two forms in one session corrupted the shared "previous directory" state: mounting via the raw binary form then unmounting via the shell function landed you in `/tmp` instead of back where you started, because the raw path never set the `AOTOOLS_VHD_PREV_DIR`/`AOTOOLS_CHD_PREV_DIR` variable the shell function reads on umount. Root cause: `aotools mount vhd`, typed at the prompt, ran the compiled binary directly -- a subprocess, which can never change its parent shell's working directory; only `mountvhd`'s shell-function wrapper could actually `cd`. Fixed by making `aotools` itself a shell function (in `shellFunctionsTemplate`, `shellinit.go`): it special-cases `mount vhd`/`umount vhd`/`mount chd`/`umount chd` to do the `cd` and track the previous directory, and passes every other subcommand straight through to the real binary unmodified. `mountvhd`/`umountvhd`/`mountchd`/`umountchd`/`mkvhd`/etc. are now thin wrappers that call this same `aotools` function, so every entry point shares one code path and one set of state variables -- there is no way left to end up in an inconsistent state by mixing spellings. Verified on the real device (using the smaller `ultima8.chd` for speed on the CHD side) with every combination: `aotools mount/umount vhd`, legacy `mountvhd`/`umountvhd`, mixed mount-via-one-form/umount-via-the-other, and the same four combinations again for `chd` -- every case correctly `cd`'d in on mount and back to the original starting directory on umount.

## Performance fix

`mount chd`'s raw-sector stripping step (`stripRawSectors`, used for MODE1/2352 and MODE2/2352 CD images) originally read/wrote one 2352-byte sector at a time with unbuffered `os.File` calls — roughly 350,000 syscall pairs for a typical CD image, which measured out to only ~1MB/s on real ARM hardware. Wrapped both sides in 1MB `bufio` buffers, which brought it up to tens of MB/s.

## Known limitation

Error paths that call `os.Exit` (via a `fatal()` helper) skip Go's deferred cleanup, so a failed `vhd resize` or archive injection can leave a scratch temp file/directory behind instead of cleaning up immediately (harmless clutter under `/tmp` or `/media/fat/linux/.mkvhd_scratch`, not data loss — nothing user-facing is corrupted). Not fixed given time constraints; worth revisiting if you hit failures often enough for it to matter.
