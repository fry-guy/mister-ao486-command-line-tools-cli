# aotools — MiSTer ao486 DOS Toolkit, single-binary edition

`aotools` is a set of command line tools for building, converting, and launching DOS and Windows 3.1 games on MiSTer's ao486 core.

This file covers installation and command usage. For build and development notes, see `NOTES.md`.

## Installation

`aotools` is a single file. It installs to its own subfolder, `/media/fat/linux/aotools/`, rather than loose inside `/media/fat/linux`, so it does not collide with the other tools already there.

1. **Copy the file.** Copy `aotools` to `/media/fat/linux/aotools/aotools` (create the folder if it does not exist) — for example, over SCP: `scp aotools root@<mister-ip>:/media/fat/linux/aotools/aotools`.

2. **Make it executable.**
   ```
   chmod +x /media/fat/linux/aotools/aotools
   ```

3. **Run the installer.**
   ```
   /media/fat/linux/aotools/aotools install
   ```
   This adds `aotools` to `$PATH` and wires up the shell functions and shortcuts (`mountvhd`, `umountvhd`, `mountchd`, `umountchd`, `mkvhd`, `resizevhd`, `mkmgl`, `mkchd`, `mkima`), then checks for the external tools and data files listed below and offers to download anything missing. Running `install` again is safe: an existing installation is detected and left alone, or upgraded automatically if it predates a newer version.

4. **Start using it.** The changes take effect immediately for any new SSH session. The current session — the one `install` ran in — does not pick them up automatically, since a shell reads `$PATH` only once, at startup. To load them into the current session without reconnecting:
   ```
   export PATH="$PATH:/media/fat/linux/aotools"
   eval "$(/media/fat/linux/aotools/aotools shellinit)"
   ```

Only steps 1–2 touch the filesystem outside `user-startup.sh` and `/etc/profile.d/aotools.sh`, aside from the dependency files below, which are only downloaded with confirmation. No existing MiSTer setup, ao486 media, or save data is affected. To reverse installation entirely, run `aotools uninstall`.

### External dependencies

Several key functionalities of `aotools` rely on external programs and data files. None are bundled: `qemu-system-i386`, `chdman`, and `mtools` are separately licensed programs, although each was specifically cross-compiled, or built and verified from scratch, for MiSTer's ARM target as part of this project. The VHD templates and boot floppy contain copyrighted Microsoft system files that cannot be redistributed.

`aotools install` can instead download all of them from known community-hosted sources, the same way installers such as `update_all.sh` handle content they cannot bundle themselves. It asks first, then requires pressing Enter to proceed or Esc to cancel before downloading.

Without an internet connection on the MiSTer itself, the table below doubles as a manual install guide: fetch each file from its listed source on another machine, then copy it to the exact `Location` path shown, via USB drive, WinSCP, or similar. `aotools doctor` confirms afterward that everything landed where it belongs.

| File | Used by | Location | Source |
|---|---|---|---|
| `mtools` | Every VHD/floppy command | `/media/fat/linux/mtools` | github.com/fry-guy/mister-ao486-command-line-tools-cli |
| `chdman` | `create chd` / `mount chd` | `/media/fat/linux/chdman` | github.com/fry-guy/mister-ao486-command-line-tools-cli |
| `qemu-system-i386` | `create vhd -dos` / `-win31` | `/media/fat/linux/qemu-system-i386` | github.com/fry-guy/mister-ao486-command-line-tools-cli |
| `qemu-bios/` (42 files) | Bundled with qemu-system-i386 | `/media/fat/linux/qemu-bios/` | same repo, `linux/qemu-bios/` |
| `disk1.img` | Drives the automated install for `create vhd -dos` | `/media/fat/games/ao486/floppy/DOS622/disk1.img` | archive.org (MS-DOS 6.22 w/ Enhanced Tools floppy set) |
| `dos_template.vhd` | Base DOS system disk for `create vhd -dos` | `/media/fat/games/ao486/dos_template.vhd` | archive.org (`dos-6.22_202607`) |
| `win31_template.vhd` | Base DOS+Windows 3.1 disk for `create vhd -win31` | `/media/fat/games/ao486/win31_template.vhd` | archive.org (`win31_template`) |

An existing ao486 DOS Toolkit setup already has every file above; `install` reports each as `[ok]` and skips the download offer. Run `aotools doctor` at any time for a read-only report on dependency status.

## Commands

Every command can be run as `aotools <command>`, or, once installed, through a shorter shell function that does the same thing.

### `create vhd` — build a new DOS/Windows 3.1 hard disk image

```
aotools create vhd <name.vhd>
aotools create vhd -dos <name.vhd> [archive|folder]
aotools create vhd -win31 <name.vhd> [archive|folder]
```
Shell shortcut: `mkvhd [-dos|-win31] <name.vhd> [archive|folder]`

- With no flag: creates a blank, unformatted `.vhd` container at a chosen size, to be formatted separately.
- `-dos`: creates the VHD, then boots an automated virtual PC to run genuine MS-DOS `FORMAT` and install DOS onto it — equivalent to a manual install, completed in 1–2 minutes.
- `-win31`: creates the VHD by cloning a pre-built DOS + Windows 3.1 install, which is faster than a real Windows 3.1 installation.
- `[archive|folder]` (optional, `-dos`/`-win31` only): a `.zip`/`.tar`/`.tar.gz`/`.tar.bz2` file, or a plain folder, containing a game. If provided, `aotools` copies its contents onto the new VHD, prompts for the launch executable if it is not obvious, and updates `AUTOEXEC.BAT` (DOS) or the Windows 3.1 startup (`WIN.INI`) to launch it automatically. Archive sources are offered for deletion afterward since the game is now on the VHD; folder sources are always left alone.
- If a size is not obvious from the source contents, a prompt requests one in MB, with a suggested value provided.

Examples:
```
aotools create vhd blank200.vhd                     # blank 200MB-ish container
aotools create vhd -dos mygame.vhd game.zip          # DOS game, ready to play
aotools create vhd -dos mygame.vhd ./game-folder     # same, from an unpacked folder
aotools create vhd -win31 mywingame.vhd wingame.zip  # Windows 3.1 game, ready to play
```

### `resize vhd` — grow or shrink an existing VHD

```
aotools resize vhd <name.vhd>
```
Shell shortcut: `resizevhd <name.vhd>`

Rebuilds `<name.vhd>` at a different size, copying every file across exactly as-is, including the boot sector and hidden/system file attributes. Useful when an existing VHD is running low on space. Reports the current size, space used, and a suggested new size, which can be accepted or overridden. Verifies the copy byte-for-byte before replacing the original, and keeps a timestamped backup unless overwriting is explicitly confirmed.

### `create mgl` — create a MiSTer game-launcher file

```
aotools create mgl -dos|-win31 "Display Name" [source_folder]
```
Shell shortcut: `mkmgl -dos|-win31 "Display Name" [source_folder]`

Scans `source_folder` (defaults to the current directory) for the `.vhd`/`.chd`/`.iso`/`.cue`/floppy image files that make up a game, and writes a `.mgl` file — the XML file MiSTer's menu uses to load content for the ao486 core — into `/media/fat/_DOS Games` or `/media/fat/_Win 3.1 Games`. If more than one candidate of a given type is found, a prompt requests a choice.

Example:
```
cd /media/fat/games/AO486/media/mygame
aotools create mgl -dos "My Game"
```

### `create chd` — compress a CD image

```
aotools create chd <input.iso|.cue|.bin|.gdi> <output.chd>
```
Shell shortcut: `mkchd <input> <output.chd>`

Compresses a raw CD image (`.iso`, or a `.cue`+`.bin` pair, etc.) into MAME's `.chd` format, which the ao486 core mounts directly, at a fraction of the size with no quality loss. For `.cue`/`.gdi`/`.toc` inputs, `aotools` locates every file the descriptor references and, once the `.chd` is created, offers to delete the now-redundant originals. Pressing Enter at the prompt keeps them by default.

Example:
```
aotools create chd MYGAME.cue mygame.chd
```

### `create diskimage` — create a floppy disk image

```
aotools create diskimage <name.ima>
aotools create diskimage <name.ima> -s <size>
aotools create diskimage <name.ima> <source>
aotools create diskimage <name.ima> <source> -s <size>
```
Shell shortcut: `mkima <name.ima> [source] [-s size]`

Creates a formatted floppy image, optionally injecting the contents of `<source>` — a folder, or a `.zip`/`.tar`/`.tar.gz`/`.tar.bz2` archive — onto it. If `-s` isn't given, `aotools` prompts with a numbered list of the standard sizes (`360k`, `720k`, `1.2m`, `1.44m`, `2.88m`); pressing Enter selects the default, `1.44m`, and if a source was given, the smallest size it fits is flagged as recommended. Passing `-s` directly skips the prompt.

### `mount vhd` / `mount chd` / `mount diskimage` — open a disk image as a folder

```
aotools mount vhd [name.vhd]            or:  mountvhd [name.vhd]
aotools umount vhd                      or:  umountvhd
aotools mount chd [name.chd]            or:  mountchd [name.chd]
aotools umount chd                      or:  umountchd
aotools mount diskimage [name.ima]
aotools umount diskimage
aotools umount                          (unmounts whichever of the above you're currently in)
```

Mounts the contents of a `.vhd`, `.chd`, or `.ima` as a real, browsable folder and changes into it automatically. Once `aotools` is installed, both forms on the `vhd`/`chd` lines above are equivalent — `aotools mount vhd`/`umount vhd`/`mount chd`/`umount chd` and their shorthand equivalents share the same underlying shell function and can be used interchangeably; `mount diskimage`/`umount diskimage` have no separate shorthand form. Pair each `mount` with the matching `umount` when finished; this returns to the original directory and cleans up.

The filename is optional. Leave it off and the matching command looks for a single `.vhd`/`.chd`/`.ima` (or `.img`) file in the current directory and mounts it automatically; if there's more than one, it lists them and asks which to use; if there's none, it says so instead of guessing. You can also point it at a folder instead of a specific file (`mountvhd somefolder/`) to search there instead of the current directory.

The type is also optional on `umount`: plain `aotools umount`, with no `vhd`/`chd`/`diskimage` after it, figures out which one you're currently sitting inside and unmounts that — no need to remember which kind of image you mounted. If you've `cd`'d away from the mount, or more than one type happens to be mounted at once, it falls back to whichever single type is actually mounted; if that's still ambiguous, it asks you to say which one explicitly.

Example:
```
cd mygamefolder          # contains exactly one .vhd
mountvhd                 # finds and mounts it automatically, now inside its contents
cp newpatch.exe .
umountvhd                # back to the original directory, unmounted
```

Editing text files (`CONFIG.SYS`, `AUTOEXEC.BAT`, etc.) with `nano` while a VHD, CHD, or disk image is mounted automatically converts them to DOS line endings (CRLF) on exit. Real MS-DOS and Windows 3.1 require CRLF, while `nano` over SSH saves plain Unix LF by default; this conversion happens without any extra step. On exit, `nano` prints `[FILENAME.EXT converted to DOS format]` for each file it converted. Files edited outside a mount are left untouched.

### `install` / `uninstall` / `shellinit` / `doctor` — setup and diagnostics

```
aotools install     # adds aotools to $PATH, wires the shell functions, offers to fetch missing dependencies
aotools uninstall    # reverses install; leaves the binary, VHDs/CHDs, and downloaded dependencies in place
aotools shellinit    # prints the shell functions (used internally by install and eval)
aotools doctor       # read-only dependency report; never downloads anything
```

`install` is described in full under Installation above. Running it again is always safe, and an older installation is upgraded automatically if needed.

`uninstall` removes only the `$PATH` and shell-function wiring `install` added. It does not delete the binary, touch any VHD/CHD files, or remove any downloaded dependency. Since the original ao486 DOS Toolkit scripts were never modified, `mountvhd`/`mkvhd`/etc. resolve back to them once the wiring is gone. Running `uninstall` again when nothing is installed is a safe no-op.

`doctor` reports on dependency status with no side effects and no download offer, for checking status at any time.

## Where things live

- The binary and this documentation: `/media/fat/linux/aotools/`
- DOS games: `/media/fat/_DOS Games`
- Windows 3.1 games: `/media/fat/_Win 3.1 Games`
- Game media (VHDs, CHDs, etc.) referenced by `.mgl` files: under `/media/fat/games/AO486/media/`
- External dependencies (qemu, chdman, templates): see the dependency table above for exact paths

No existing files, folders, or games are touched by installing or running `aotools` unless a command is explicitly pointed at them — for example, `resize vhd somegame.vhd` modifies `somegame.vhd` by design.
