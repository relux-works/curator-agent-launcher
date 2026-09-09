"""Real controlling-terminal regression; only synthetic fake ax/provider."""
import os, sys, pty, signal, select, time, tempfile, pathlib, re

with tempfile.TemporaryDirectory(prefix='execution-pty-') as directory:
    helper = pathlib.Path(directory) / 'fake-ax'
    helper.write_text('#!' + sys.executable + '\n' + '''import os, signal, sys
count = 0
def hit(sig, frame):
 global count
 count += 1
 os.write(1, b'INT\\n')
signal.signal(signal.SIGINT, hit)
signal.signal(signal.SIGTSTP, signal.SIG_DFL)
# Ax stdin is a document; terminal interaction uses its controlling tty.
tty = open('/dev/tty', 'r') if len(sys.argv) > 1 else sys.stdin
os.write(1, ('READY child=%d\\n' % os.getpid()).encode())
while True:
 line = tty.readline().strip()
 if line == 'check': os.write(1, ('COUNT=%d\\n' % count).encode())
 elif line == 'read': os.write(1, b'READ_OK\\n')
 elif line == 'quit': break
''')
    helper.chmod(0o700)
    for trial in range(5):
        pid, fd = pty.fork()
        if pid == 0:
            os.execve(sys.argv[1], [sys.argv[1], str(helper), directory, sys.argv[2], 'pty'], {'PATH': '/usr/bin:/bin'})
        data = b''
        reaped = False
        helper_group = None
        def until(token):
            global data
            deadline = time.monotonic() + 5
            while token not in data and time.monotonic() < deadline:
                if select.select([fd], [], [], .05)[0]:
                    try: block = os.read(fd, 4096)
                    except OSError: break
                    if not block: break
                    data += block
            assert token in data, (token, data)
        try:
            until(b'READY child=')
            helper_group = int(re.search(rb'READY child=(\d+)', data).group(1))
            child_group = os.tcgetpgrp(fd)
            assert child_group == helper_group and child_group != pid, ('child lacks separate foreground ownership', data)
            os.write(fd, b'\x03')
            until(b'INT\r\n')
            time.sleep(.15)  # Allow any erroneous duplicate relay to arrive.
            os.write(fd, b'check\n')
            until(b'COUNT=1\r\n')
            assert data.count(b'INT\r\n') == 1, data
            os.kill(pid, signal.SIGINT)  # Parent-PID-only delivery must still relay.
            time.sleep(.15)
            os.write(fd, b'check\n')
            until(b'COUNT=2\r\n')
            assert data.count(b'INT\r\n') == 2, data
            os.write(fd, b'read\n')
            until(b'READ_OK')
            os.write(fd, b'\x1a')  # Real terminal VSUSP to foreground child.
            deadline = time.monotonic() + 5
            stopped = False
            while time.monotonic() < deadline:
                got, status = os.waitpid(pid, os.WNOHANG | os.WUNTRACED)
                if got:
                    stopped = os.WIFSTOPPED(status)
                    break
                time.sleep(.01)
            assert stopped, ('launcher did not stop with child', data)
            assert os.tcgetpgrp(fd) == pid, 'terminal not restored on stop'
            os.kill(pid, signal.SIGCONT)
            data = b''
            os.write(fd, b'read\n')
            until(b'READ_OK')
            assert os.tcgetpgrp(fd) == child_group, 'child not foreground after resume'
            os.write(fd, b'quit\n')
            until(b'RESTORED=true')
            _, status = os.waitpid(pid, 0)
            reaped = True
            assert os.WIFEXITED(status) and os.WEXITSTATUS(status) == 0, status
            print('trial=%d: one terminal INT + one parent-only INT, foreground read, stop/resume/read, restored terminal, exit=0' % (trial+1), flush=True)
        finally:
            if not reaped:
                # Kill both isolated groups; never touch the host terminal group.
                if helper_group is not None and helper_group > 0 and helper_group not in (pid, os.getpgrp()):
                    try: os.killpg(helper_group, signal.SIGKILL)
                    except ProcessLookupError: pass
                try: os.kill(pid, signal.SIGKILL)
                except ProcessLookupError: pass
                try: os.waitpid(pid, 0)
                except ChildProcessError: pass
            os.close(fd)
