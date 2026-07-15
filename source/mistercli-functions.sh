# mistercli-functions.sh -- thin shell wrapper around `mistercli
# mount`/`umount`. A subprocess (mistercli itself) can never change
# its parent shell's own working directory, so the actual `cd` into
# and out of the mounted volume has to happen here, in functions
# sourced into your interactive shell -- exactly like the original
# mountvhd/umountvhd/mountchd/umountchd scripts, which also had to be
# sourced (". mountvhd ...") for the same reason.
#
# Install once, e.g. by adding this to /media/fat/linux/user-startup.sh
# or your shell's profile:
#   . /media/fat/linux/mistercli/mistercli-functions.sh
#
# Then use exactly like the original tools:
#   mountvhd game.vhd   (cd's into it)
#   umountvhd            (cd's back out, unmounts)
#   mountchd game.chd
#   umountchd

MISTERCLI="${MISTERCLI:-/media/fat/linux/mistercli/mistercli}"

mountvhd() {
    local mp
    mp="$("$MISTERCLI" mount vhd "$1")" || return 1
    MISTERCLI_VHD_PREV_DIR="$PWD"
    cd "$mp" || return 1
}

umountvhd() {
    if [ -n "$MISTERCLI_VHD_PREV_DIR" ] && [ -d "$MISTERCLI_VHD_PREV_DIR" ]; then
        cd "$MISTERCLI_VHD_PREV_DIR"
    else
        cd /tmp
    fi
    "$MISTERCLI" umount vhd
    unset MISTERCLI_VHD_PREV_DIR
}

mountchd() {
    local mp
    mp="$("$MISTERCLI" mount chd "$1")" || return 1
    MISTERCLI_CHD_PREV_DIR="$PWD"
    cd "$mp" || return 1
}

umountchd() {
    if [ -n "$MISTERCLI_CHD_PREV_DIR" ] && [ -d "$MISTERCLI_CHD_PREV_DIR" ]; then
        cd "$MISTERCLI_CHD_PREV_DIR"
    else
        cd /media/fat
    fi
    "$MISTERCLI" umount chd
    unset MISTERCLI_CHD_PREV_DIR
}

# Optional convenience aliases matching the original standalone tool
# names, for muscle memory. These are plain functions (not scripts),
# so they run with no `. ` sourcing needed as long as this file itself
# was sourced into your shell.
mkvhd() {
    if [ "$1" = "-dos" ] || [ "$1" = "-win31" ]; then
        local flag="$1"; shift
        "$MISTERCLI" vhd create "$flag" "$@"
    else
        "$MISTERCLI" vhd create "$@"
    fi
}
resizevhd() { "$MISTERCLI" vhd resize "$@"; }
mkmgl()     { "$MISTERCLI" mgl create "$@"; }
mkchd()     { "$MISTERCLI" chd create "$@"; }
mkima()     { "$MISTERCLI" ima create "$@"; }
