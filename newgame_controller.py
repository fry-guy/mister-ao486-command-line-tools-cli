#!/usr/bin/env python3
"""
ao486 VHD auto-creator v4
Fixes: explicit disk geometry, XCOPY expansion, robust prompt detection
(byte-level non-blocking reads instead of readline(), correct pattern
ordering so the FORMAT Y/N confirmation is actually answered, and honest
success/failure return value instead of always returning True).
"""
import subprocess, socket, json, time, sys, os, threading, queue, signal, fcntl

QEMU = "/media/fat/linux/qemu-system-i386"
QEMU_BIOS_DIR = "/media/fat/linux/qemu-bios"
QMP_SOCK = "/tmp/ao486-qmp.sock"

def calc_geometry(vhd_path):
    """Calculate CHS geometry for a VHD file"""
    size = os.path.getsize(vhd_path)
    total_sectors = size // 512
    heads = 16
    spt = 63
    cyls = total_sectors // (heads * spt)
    return cyls, heads, spt

class QEMUController:
    def __init__(self):
        self.proc = None
        self.qmp = None
        self.output = queue.Queue()

    def start(self, vhd, floppy, bootref):
        if os.path.exists(QMP_SOCK):
            os.remove(QMP_SOCK)

        cyls, heads, spt = calc_geometry(vhd)
        print(f"[*] VHD geometry: {cyls}/{heads}/{spt}", flush=True)

        cmd = [
            QEMU,
            "-L", QEMU_BIOS_DIR,
            "-nographic",
            "-no-reboot",
            "-m", "32",
            "-drive", f"file={floppy},format=raw,if=floppy",
            "-drive", f"file={vhd},format=raw,if=none,id=maindisk,media=disk",
            "-device", f"ide-hd,drive=maindisk,bus=ide.0,unit=0,cyls={cyls},heads={heads},secs={spt}",
            "-drive", f"file={bootref},format=raw,if=none,id=bootref,media=disk",
            "-device", "ide-hd,drive=bootref,bus=ide.0,unit=1",
            "-boot", "order=a",
            "-qmp", f"unix:{QMP_SOCK},server,nowait",
            "-serial", "stdio",
            "-monitor", "none",
        ]
        print(f"[*] Starting QEMU...", flush=True)
        self.proc = subprocess.Popen(
            cmd, stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
            stdin=subprocess.DEVNULL
        )

        # Put the pipe in non-blocking mode so we can read partial output
        # (prompts like "Proceed with Format (Y/N)?" have NO trailing
        # newline, so a readline()-based reader blocks forever on them)
        fd = self.proc.stdout.fileno()
        fl = fcntl.fcntl(fd, fcntl.F_GETFL)
        fcntl.fcntl(fd, fcntl.F_SETFL, fl | os.O_NONBLOCK)

        def reader():
            while True:
                if self.proc.poll() is not None:
                    break
                try:
                    chunk = os.read(fd, 4096)
                except BlockingIOError:
                    time.sleep(0.1)
                    continue
                except OSError:
                    break
                if not chunk:
                    time.sleep(0.1)
                    continue
                text = chunk.decode('utf-8', errors='replace')
                print(text, end='', flush=True)
                self.output.put(text)
        threading.Thread(target=reader, daemon=True).start()

    def connect_qmp(self, timeout=60):
        print("[*] Connecting to QMP...", flush=True)
        deadline = time.time() + timeout
        while time.time() < deadline:
            try:
                s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
                s.connect(QMP_SOCK)
                s.settimeout(5)
                s.recv(4096)
                s.send(json.dumps({"execute": "qmp_capabilities"}).encode() + b'\n')
                s.recv(4096)
                self.qmp = s
                print("[*] QMP connected", flush=True)
                return True
            except (FileNotFoundError, ConnectionRefusedError, OSError):
                time.sleep(0.5)
        return False

    def send_key(self, key, shift=False):
        mods = []
        if shift:
            mods.append({"type": "key", "data": {"down": True, "key": {"type": "qcode", "data": "shift"}}})
        mods += [
            {"type": "key", "data": {"down": True,  "key": {"type": "qcode", "data": key}}},
            {"type": "key", "data": {"down": False, "key": {"type": "qcode", "data": key}}},
        ]
        if shift:
            mods.append({"type": "key", "data": {"down": False, "key": {"type": "qcode", "data": "shift"}}})
        cmd = json.dumps({"execute": "input-send-event", "arguments": {"events": mods}})
        try:
            self.qmp.send(cmd.encode() + b'\n')
        except:
            pass
        time.sleep(0.15)

    def send_string(self, text):
        keymap = {
            '\n': ('ret', False), '\r': ('ret', False),
            ' ': ('spc', False), '\\': ('backslash', False),
            '/': ('slash', False), ':': ('semicolon', True),
            '.': ('dot', False), '-': ('minus', False),
        }
        for ch in text:
            if ch in keymap:
                key, shift = keymap[ch]
            elif ch.isupper():
                key, shift = ch.lower(), True
            elif ch.isdigit():
                key, shift = ch, False
            else:
                key, shift = ch.lower(), False
            self.send_key(key, shift)

    def wait_for(self, patterns, timeout=300, window=4000):
        """Accumulate a rolling text buffer (not discrete lines) and check
        for pattern matches, so prompts without a trailing newline are
        still detected."""
        if isinstance(patterns, str):
            patterns = [patterns]
        deadline = time.time() + timeout
        acc = ""
        while time.time() < deadline:
            try:
                chunk = self.output.get(timeout=2)
                acc += chunk
                if len(acc) > window:
                    acc = acc[-window:]
                for p in patterns:
                    if p.lower() in acc.lower():
                        return p
            except queue.Empty:
                if self.proc and self.proc.poll() is not None:
                    print(f"[*] QEMU exited with code {self.proc.returncode}", flush=True)
                    return "QEMU_EXITED"
        return None

    def shutdown(self):
        try:
            self.qmp.send(json.dumps({"execute": "quit"}).encode() + b'\n')
        except:
            pass
        if self.proc:
            try:
                self.proc.wait(timeout=10)
            except subprocess.TimeoutExpired:
                self.proc.kill()

def run(vhd, floppy, bootref):
    ctrl = QEMUController()

    def sigint(sig, frame):
        print("\n[!] Interrupted", flush=True)
        ctrl.shutdown()
        sys.exit(1)
    signal.signal(signal.SIGINT, sigint)

    ctrl.start(vhd, floppy, bootref)

    if not ctrl.connect_qmp(timeout=60):
        print("[!] Failed to connect to QMP", flush=True)
        ctrl.shutdown()
        return False

    print("[*] Waiting for FORMAT confirmation prompt...", flush=True)
    result = ctrl.wait_for([
        "Proceed with Format",
        "Invalid drive",
        "QEMU_EXITED"
    ], timeout=120)

    if result is None:
        print("[!] Timed out waiting for FORMAT prompt", flush=True)
        ctrl.shutdown()
        return False

    if result == "QEMU_EXITED":
        print("[!] QEMU exited unexpectedly before reaching FORMAT prompt", flush=True)
        return False

    if "invalid drive" in result.lower():
        print("[!] DOS cannot see the VHD - geometry issue persists", flush=True)
        ctrl.shutdown()
        return False

    # Handle FORMAT confirmation
    print("[*] Sending Y to FORMAT...", flush=True)
    time.sleep(1)
    ctrl.send_string("Y\n")

    print("[*] Waiting for setup completion...", flush=True)
    result = ctrl.wait_for([
        "AO486 SETUP COMPLETE",
        "SETUP COMPLETE DEBUG",
        "SETUP COMPLETE",
        "QEMU_EXITED"
    ], timeout=300)

    time.sleep(2)
    ctrl.shutdown()

    if result is None:
        print("[!] Timed out waiting for setup completion", flush=True)
        return False
    if result == "QEMU_EXITED":
        print("[!] QEMU exited before setup completed", flush=True)
        return False

    return True

if __name__ == '__main__':
    if len(sys.argv) != 4:
        print(f"Usage: {sys.argv[0]} <vhd> <floppy.img> <dos_template.vhd>")
        sys.exit(1)
    vhd, floppy, bootref = sys.argv[1], sys.argv[2], sys.argv[3]
    for f in [vhd, floppy, bootref]:
        if not os.path.exists(f):
            print(f"[!] File not found: {f}")
            sys.exit(1)
    sys.exit(0 if run(vhd, floppy, bootref) else 1)
