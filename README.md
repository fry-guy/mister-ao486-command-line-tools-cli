# aotools — MiSTer ao486 DOS Toolkit, single-binary edition

`aotools` is a single-file replacement for the ao486 DOS Toolkit scripts (`mkvhd`, `resizevhd`, `mkmgl`, `mkchd`, `mkima`, `mountvhd`, `umountvhd`, `mountchd`, `umountchd`, and the Python helpers behind them). One executable performs everything those separate scripts did, with no Python and no compiler required to run it.

This file covers installation and command usage. For build and development notes, see `INSTALL.md`.

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

`aotools` still relies on the same external programs and data files the original scripts required. None are bundled: `qemu-system-i386`, `chdman`, and `mtools` are separately licensed programs; the VHD templates and boot floppy contain copyrighted Microsoft system files that cannot be redistributed.

`aotools install` can download all of them from known community-hosted sources, the same way installers such as `update_all.sh` handle content they cannot bundle themselves. It asks first, then requires pressing Enter to proceed or Esc to cancel before downloading anything.

| File | Used by | Location | Source |
|---|---|---|---|
| `mtools` | Every VHD/floppy command | `/media/fat/linux/mtools` | github.com/fry-guy/mister-ao486-command-line-tools-cli |
| `chdman` | `chd create` / `mount chd` | `/media/fat/linux/chdman` | github.com/fry-guy/mister-ao486-command-line-tools-cli |
| `qemu-system-i386` | `vhd create -dos` / `-win31` | `/media/fat/linux/qemu-system-i386` | github.com/fry-guy/mister-ao486-command-line-tools-cli |
| `qemu-bios/` (42 files) | Bundled with qemu-system-i386 | `/media/fat/linux/qemu-bios/` | same repo, `linux/qemu-bios/` |
| `disk1.img` | Drives the automated install for `vhd create -dos` | `/media/fat/games/ao486/floppy/DOS622/disk1.img` | archive.org (MS-DOS 6.22 w/ Enhanced Tools floppy set) |
| `dos_template.vhd` | Base DOS system disk for `vhd create -dos` | `/media/fat/games/ao486/dos_template.vhd` | archive.org (`dos-6.22_202607`) |
| `win31_template.vhd` | Base DOS+Windows 3.1 disk for `vhd create -win31` | `/media/fat/games/ao486/win31_template.vhd` | archive.org (`win31_template`) |

An existing ao486 DOS Toolkit setup already has every file above; `install` reports each as `[ok]` and skips the download offer. Run `aotools doctor` at any time for a read-only report on dependency status.

## Commands

Every command can be run as `aotools <command>`, or, once installed, through a shorter shell function that does the same thing.

### `vhd create` — build a new DOS/Windows 3.1 hard disk image

```
aotools vhd create <name.vhd>
aotools vhd create -dos <name.vhd> [archive]
aotools vhd create -win31 <name.vhd> [archive]
```
Shell shortcut: `mkvhd [-dos|-win31] <name.vhd> [archive]`

- With no flag: creates a blank, unformatted `.vhd` container at a chosen size, to be formatted separately.
- `-dos`: creates the VHD, then boots an automated virtual PC to run genuine MS-DOS `FORMAT` and install DOS onto it — equivalent to a manual install, completed in 1–2 minutes.
- `-win31`: creates the VHD by cloning a pre-built DOS + Windows 3.1 install, which is faster than a real Windows 3.1 installation.
- `[archive]` (optional, `-dos`/`-win31` only): a `.zip`/`.tar`/`.tar.gz`/`.tar.bz2` file containing a game. If provided, `aotools` extracts it onto the new VHD, prompts for the launch executable if it is not obvious, and updates `AUTOEXEC.BAT` (DOS) or the Windows 3.1 startup (`WIN.INI`) to launch it automatically.
- If a size is not obvious from the archive contents, a prompt requests one in MB, with a suggested value provided.

Examples:
```
aotools vhd create blank200.vhd                     # blank 200MB-ish container
aotools vhd create -dos mygame.vhd game.zip          # DOS game, ready to play
aotools vhd create -win31 mywingame.vhd wingame.zip  # Windows 3.1 game, ready to play
```

### `vhd resize` — grow an existing VHD

```
aotools vhd resize <name.vhd>
```
Shell shortcut: `resizevhd <name.vhd>`

Rebuilds `<name.vhd>` at a larger size, copying every file across exactly as-is, including the boot sector and hidden/system file attributes. Useful when an existing VHD is running low on space. Reports the current size, space used, and a suggested new size, which can be accepted or overridden. Verifies the copy byte-for-byte before replacing the original, and keeps a timestamped backup unless overwriting is explicitly confirmed.

### `mgl create` — create a MiSTer game-launcher file

```
aotools mgl create -dos|-win31 "Display Name" [source_folder]
```
Shell shortcut: `mkmgl -dos|-win31 "Display Name" [source_folder]`

Scans `source_folder` (defaults to the current directory) for the `.vhd`/`.chd`/`.iso`/`.cue`/floppy image files that make up a game, and writes a `.mgl` file — the XML file MiSTer's menu uses to load content for the ao486 core — into `/media/fat/_DOS Games` or `/media/fat/_Win 3.1 Games`. If more than one candidate of a given type is found, a prompt requests a choice.

Example:
```
cd /media/fat/games/AO486/media/mygame
aotools mgl create -dos "My Game"
```

### `chd create` — compress a CD image

```
aotools chd create <input.iso|.cue|.bin|.gdi> <output.chd>
```
Shell shortcut: `mkchd <input> <output.chd>`

Compresses a raw CD image (`.iso`, or a `.cue`+`.bin` pair, etc.) into MAME's `.chd` format, which the ao486 core mounts directly, at a fraction of the size with no quality loss. For `.cue`/`.gdi`/`.toc` inputs, `aotools` locates every file the descriptor references and, once the `.chd` is created, offers to delete the now-redundant originals. Pressing Enter at the prompt keeps them by default.

Example:
```
aotools chd create MYGAME.cue mygame.chd
```

### `ima create` — create a floppy disk image

```
aotools ima create <name.ima>
aotools ima create <name.ima> -s <size>
aotools ima create <name.ima> <source>
aotools ima create <name.ima> <source> -s <size>
```
Shell shortcut: `mkima <name.ima> [source] [-s size]`

Creates a blank, formatted floppy image (1.44MB by default), or one sized to fit `<source>` — a folder, or an archive to be extracted onto it — if provided. Valid `-s` sizes: `360k`, `720k`, `1.2m`, `1.44m`, `2.88m`.

### `mount vhd` / `mount chd` — open a disk image as a folder

```
aotools mount vhd <name.vhd>      or:  mountvhd <name.vhd>
aotools umount vhd                or:  umountvhd
aotools mount chd <name.chd>      or:  mountchd <name.chd>
aotools umount chd                or:  umountchd
```

Mounts the contents of a `.vhd` or `.chd` as a real, browsable folder and changes into it automatically. Once `aotools` is installed, both forms on each line above are equivalent — `aotools mount vhd`/`umount vhd`/`mount chd`/`umount chd` and their shorthand equivalents share the same underlying shell function and can be used interchangeably. Pair each `mount` with the matching `umount` when finished; this returns to the original directory and cleans up.

Example:
```
mountvhd mygame.vhd      # now inside the VHD's contents
cp newpatch.exe .
umountvhd                # back to the original directory, unmounted
```

Editing text files (`CONFIG.SYS`, `AUTOEXEC.BAT`, etc.) with `nano` while a VHD or CHD is mounted automatically converts them to DOS line endings (CRLF) on exit. Real MS-DOS and Windows 3.1 require CRLF, while `nano` over SSH saves plain Unix LF by default; this conversion happens without any extra step. A confirmation message is printed after conversion. Files edited outside a mount are left untouched.

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

No existing files, folders, or games are touched by installing or running `aotools` unless a command is explicitly pointed at them — for example, `vhd resize somegame.vhd` modifies `somegame.vhd` by design.
