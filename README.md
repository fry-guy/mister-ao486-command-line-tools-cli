# aotools — MiSTer ao486 DOS Toolkit, single-binary edition

`aotools` offers a set of command-line tools for building, converting, and launching DOS games on the MiSTer's ao486 core.

This readme is a plain-English user guide: what to install, in what order, and exactly what each command does and expects. For build/development notes (rebuilding from source, what was tested, bugs found along the way), see `INSTALL.md`.

## Where things go on your MiSTer

`aotools` itself is a single file: `/media/fat/linux/aotools/aotools`. It is **not** dropped loose directly into `/media/fat/linux` — it lives in its own `aotools` subfolder underneath it, so it can't collide with any of the other tools/scripts already in `/media/fat/linux`. Create that subfolder if it doesn't exist yet. There is no second file (like the old `aotools-functions.sh`) to install alongside it — the shell functions are built into the one binary (see `install`/`shellinit` below).

## What `aotools` needs that it doesn't provide

`aotools` is one file, but it isn't magic — it still drives the same external programs and data files the original scripts always needed. None of these are bundled inside `aotools` itself, for two different reasons: `qemu-system-i386`/`chdman`/`mtools` are separately-built, separately-licensed programs (embedding them would balloon the binary and raise licensing questions that aren't `aotools`'s to answer); the VHD templates and DOS boot floppy contain real, copyrighted Microsoft system files that no tool has the right to redistribute bundled inside itself.

That said, **`aotools install` can now offer to download every one of them for you**, from known community-hosted copies, the same way community MiSTer installers like `update_all.sh` handle content they can't bundle themselves: it asks first, shows you exactly what it's about to fetch and where from, requires you to explicitly type `I agree` acknowledging these are third-party copyrighted files (not `aotools`'s own), and only then downloads. Nothing is fetched silently or automatically.

| File | What it's for | Goes where on your MiSTer | Source `aotools install` downloads it from |
|---|---|---|---|
| `mtools` | Every VHD/floppy command | `/media/fat/linux/mtools` | github.com/fry-guy/mister-ao486-command-line-tools-cli |
| `chdman` | `chd create` / `mount chd` | `/media/fat/linux/chdman` | github.com/fry-guy/mister-ao486-command-line-tools-cli |
| `qemu-system-i386` | `vhd create -dos`/`-win31` | `/media/fat/linux/qemu-system-i386` | github.com/fry-guy/mister-ao486-command-line-tools-cli |
| `qemu-bios/` folder (42 files) | Bundled alongside qemu-system-i386, not separate | `/media/fat/linux/qemu-bios/` | same repo, `linux/qemu-bios/` |
| `disk1.img` | Drives the automated DOS install for `vhd create -dos` | `/media/fat/games/ao486/floppy/DOS622/disk1.img` | archive.org (MS-DOS 6.22 w/ Enhanced Tools floppy set) |
| `dos_template.vhd` | Pre-built DOS system disk `vhd create -dos` builds on | `/media/fat/games/ao486/dos_template.vhd` | archive.org (`dos-6.22_202607`) |
| `win31_template.vhd` | Pre-built DOS+Windows 3.1 disk `vhd create -win31` clones | `/media/fat/games/ao486/win31_template.vhd` | archive.org (`win31_template`) |

You never have to deal with any of this if you already have a working ao486 DOS Toolkit setup (you've made DOS/Windows 3.1 games before) — you already have every file above, and `aotools install` will simply report everything as `[ok]` and skip the download offer entirely.

Run `aotools doctor` at any time for a plain, read-only report on exactly what's present and what's missing (it never offers to download anything — that's only ever part of `install`, on request).

## Step-by-step install

1. **Copy the file.** Copy the single `aotools` binary to `/media/fat/linux/aotools/aotools` on your MiSTer (create the `aotools` folder if it doesn't exist). This is the only file you need — over SFTP/SCP, or `scp aotools root@<mister-ip>:/media/fat/linux/aotools/aotools`.

2. **Make it executable.** Over SSH:
   ```
   chmod +x /media/fat/linux/aotools/aotools
   ```

3. **Install.**
   ```
   /media/fat/linux/aotools/aotools install
   ```
   This does two things in one step:
   - Adds one line to `/media/fat/linux/user-startup.sh` (MiSTer's own boot script) so `mountvhd`, `umountvhd`, `mountchd`, `umountchd`, and the `mkvhd`/`resizevhd`/`mkmgl`/`mkchd`/`mkima` shortcuts are available in every SSH session from now on. Safe to run more than once — it checks first and won't add a duplicate line.
   - Checks for `qemu-system-i386`, `chdman`, `mtools`, the VHD templates, and the boot floppy (see the table above), and if anything's missing, offers to download it for you right then (with the copyright acknowledgment step described above). Decline and it just skips that part; accept and it fetches only what's actually missing.

4. **Start using it.** Either reboot the MiSTer once, or, to use the commands in your *current* SSH session immediately without rebooting:
   ```
   eval "$(/media/fat/linux/aotools/aotools shellinit)"
   ```
   After a reboot (or that one `eval` line), you can just type `mountvhd`, `mkvhd`, etc. directly — see the command reference below.

That's the entire install. Steps 1–2 are the only ones that touch the filesystem beyond `user-startup.sh` (and, only if you say yes to the download offer, the dependency files in the table above); nothing about your existing MiSTer setup, ao486 media, or save games is touched. Changed your mind? `aotools uninstall` reverses it completely — see below.

## Command reference

Every command can be run two ways: the long form, `aotools <command>`, or (once you've done step 4/5 above) a short shell function. Both do exactly the same thing — the short forms just save typing.

### `vhd create` — build a new DOS/Windows 3.1 hard disk image

```
aotools vhd create <name.vhd>
aotools vhd create -dos <name.vhd> [archive]
aotools vhd create -win31 <name.vhd> [archive]
```
Shell shortcut: `mkvhd [-dos|-win31] <name.vhd> [archive]`

- With no flag: creates a blank, unformatted `.vhd` container of a size you choose. You'd format it yourself.
- `-dos`: creates the VHD, then boots a real (invisible, automated) virtual PC to run genuine MS-DOS `FORMAT` and install DOS onto it — the same as if you'd done it by hand, just automated. Takes 1–2 minutes.
- `-win31`: creates the VHD by cloning a pre-built, already-working DOS + Windows 3.1 install (much faster than a real Windows 3.1 install, since that's already done for you).
- `[archive]` (optional, `-dos`/`-win31` only): a `.zip`/`.tar`/`.tar.gz`/`.tar.bz2` file containing a game. If given, `aotools` extracts it onto the new VHD, asks you to pick which file inside it actually launches the game (if that isn't obvious), and wires up `AUTOEXEC.BAT` (DOS) or the Windows 3.1 startup (`WIN.INI`) to launch it automatically.
- You'll be prompted for a size in MB if one isn't obvious from the archive contents; a suggested size is calculated for you.

Examples:
```
aotools vhd create blank200.vhd                     # blank 200MB-ish container, you format it
aotools vhd create -dos mygame.vhd game.zip          # DOS game, ready to play
aotools vhd create -win31 mywingame.vhd wingame.zip  # Windows 3.1 game, ready to play
```

### `vhd resize` — grow an existing VHD

```
aotools vhd resize <name.vhd>
```
Shell shortcut: `resizevhd <name.vhd>`

Rebuilds `<name.vhd>` at a larger size, copying every file across exactly as-is (boot sector, hidden/system file attributes, and all). Useful when a VHD you already made is running low on space. Shows you the current size, how much space is actually used, and a suggested new size; you can accept the suggestion or type your own. Verifies the copy byte-for-byte before replacing the original, and keeps a timestamped backup of the original unless you explicitly confirm overwriting it.

### `mgl create` — make a MiSTer game-launcher file

```
aotools mgl create -dos|-win31 "Display Name" [source_folder]
```
Shell shortcut: `mkmgl -dos|-win31 "Display Name" [source_folder]`

Scans `source_folder` (defaults to the current directory) for the `.vhd`/`.chd`/`.iso`/`.cue`/floppy image files that make up a game, and writes a `.mgl` file — the small XML file MiSTer's menu uses to know what to load for the ao486 core — into `/media/fat/_DOS Games` or `/media/fat/_Win 3.1 Games`. If more than one candidate of a given type is found, you're asked which one to use.

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

Compresses a raw CD image (`.iso`, or a `.cue`+`.bin` pair, etc.) into MAME's `.chd` format, which is what the ao486 core actually mounts — much smaller than the raw image, no quality lost. For `.cue`/`.gdi`/`.toc` inputs, `aotools` finds every file the descriptor references (e.g. the `.bin` a `.cue` points at) and, once the `.chd` is made, offers to delete the now-redundant originals — it always asks first and defaults to keeping them if you just press Enter.

Example:
```
aotools chd create MYGAME.cue mygame.chd
```

### `ima create` — make a floppy disk image

```
aotools ima create <name.ima>
aotools ima create <name.ima> -s <size>
aotools ima create <name.ima> <source>
aotools ima create <name.ima> <source> -s <size>
```
Shell shortcut: `mkima <name.ima> [source] [-s size]`

Creates a blank, formatted floppy image (1.44MB by default), or one sized to fit `<source>` (a folder, or an archive that gets extracted onto it) if given. Valid `-s` sizes: `360k`, `720k`, `1.2m`, `1.44m`, `2.88m`.

### `mount vhd` / `mount chd` — open a disk image as a real folder

```
aotools mount vhd <name.vhd>      or:  mountvhd <name.vhd>
aotools umount vhd                or:  umountvhd
aotools mount chd <name.chd>      or:  mountchd <name.chd>
aotools umount chd                or:  umountchd
```

Mounts the contents of a `.vhd` or `.chd` as a real, browsable/editable folder, so you can drop files in or pull them out without any special tools. **Use the shell function form (`mountvhd`/`mountchd`), not `aotools mount vhd` directly** — a program can never change its parent shell's current directory, so only the shell function versions actually `cd` you into the mounted folder; the raw `aotools mount vhd` form only prints the mountpoint path. Always pair a `mount` with the matching `umount` when you're done, which `cd`'s you back out and cleans up.

Example:
```
mountvhd mygame.vhd      # you're now inside the VHD's contents
cp newpatch.exe .
umountvhd                # back to where you were, unmounted
```

### `install` / `uninstall` / `shellinit` / `doctor` — setup and diagnostics

```
aotools install     # one-time: wires the shell functions in AND offers to fetch missing deps
aotools uninstall   # reverses exactly what install did to user-startup.sh -- nothing else
aotools shellinit   # prints the shell functions (used internally by install/eval)
aotools doctor      # read-only report on qemu/chdman/templates/etc. -- never downloads anything
```

You generally only need `install` once (see "Step-by-step install") — it wires up the shell functions and, if anything from the dependency table is missing, offers to download it (always asking first, always requiring the copyright acknowledgment). Run `doctor` any time afterward, with no side effects at all, whenever something isn't behaving as expected and you want to rule out a missing dependency.

If you ever want to go back to exactly how things were before `aotools`, run `aotools uninstall`. It removes the one block `install` added to `/media/fat/linux/user-startup.sh` and nothing else — it does not delete the `aotools` binary, does not touch any VHD/CHD/game files you created with it, and does not remove any dependency file `install`'s download offer may have fetched for you. Since `aotools` never modified or removed the original ao486 DOS Toolkit scripts in the first place, `mountvhd`/`mkvhd`/etc. simply go back to resolving to those original scripts as soon as the wiring is gone (next reboot/new shell — or immediately in your current shell with `unset -f mountvhd umountvhd mountchd umountchd mkvhd resizevhd mkmgl mkchd mkima` if you'd already loaded them). Running `uninstall` again afterward (or when nothing was installed) is a safe no-op — it just tells you there's nothing to remove.

## Where things live

- The binary and this documentation: `/media/fat/linux/aotools/`
- DOS games: `/media/fat/_DOS Games`
- Windows 3.1 games: `/media/fat/_Win 3.1 Games`
- Game media (VHDs, CHDs, etc.) referenced by `.mgl` files: under `/media/fat/games/AO486/media/`
- Everything `aotools` reads but doesn't ship (qemu, chdman, templates): see the dependency table above for exact paths.

None of your existing files, folders, or games are touched by installing or running `aotools` unless you explicitly point a command at them (e.g. `vhd resize somegame.vhd` will, by design, modify `somegame.vhd`).
