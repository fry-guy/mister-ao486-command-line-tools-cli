# This standalone file is no longer needed / no longer part of the
# deliverable. The mountvhd/umountvhd/mountchd/umountchd/mkvhd/
# resizevhd/mkmgl/mkchd/mkima shell functions are now embedded
# directly inside the aotools binary itself (see shellinit.go in the
# Go source). To wire them into your shell, run:
#
#   aotools install
#
# This appends one line to /media/fat/linux/user-startup.sh so it
# happens automatically on every future boot/shell. It's idempotent
# (safe to run more than once) and won't touch anything else in that
# file. To load the functions into just your CURRENT shell without
# rebooting:
#
#   eval "$(/media/fat/linux/aotools/aotools shellinit)"
#
# See INSTALL.md.
