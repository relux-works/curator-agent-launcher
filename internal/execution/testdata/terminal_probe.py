"""Real controlling-terminal regression; only synthetic fake ax/provider."""
import os, sys, pty, signal, select, time, tempfile, pathlib, re

with tempfile.TemporaryDirectory(prefix='execution-pty-') as directory:
    helper = pathlib.Path(directory) / 'fake-ax'
    helper.write_text('#!' + sys.executable + '\n' + '''import os, select, signal, sys
count = 0
# Buffered readline may restart a canonical read while a Python signal handler
# is pending. A wakeup fd makes signal receipt an explicit select event.
signal_read, signal_write = os.pipe()
os.set_blocking(signal_write, False)
signal.set_wakeup_fd(signal_write)
def hit(sig, frame):
 global count
 count += 1
signal.signal(signal.SIGINT, hit)
signal.signal(signal.SIGTSTP, signal.SIG_DFL)
# Ax stdin is a document; terminal interaction uses its controlling tty.
tty = open('/dev/tty', 'r') if len(sys.argv) > 1 else sys.stdin
os.write(1, ('READY child=%d\\n' % os.getpid()).encode())
while True:
 ready, _, _ = select.select([tty, signal_read], [], [])
 if signal_read in ready:
  events = os.read(signal_read, 4096)
  for event in events:
   if event == signal.SIGINT: os.write(1, b'INT\\n')
 if tty not in ready: continue
 line = os.read(tty.fileno(), 4096).decode().strip()
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
            os.write(fd, b'check\n')
            until(b'COUNT=1\r\n')
            assert data.count(b'INT\r\n') == 1, data
            data = b''
            os.kill(pid, signal.SIGINT)  # Parent-PID-only delivery must still relay.
            until(b'INT\r\n')
            os.write(fd, b'check\n')
            until(b'COUNT=2\r\n')
            assert data.count(b'INT\r\n') == 1, data
            os.write(fd, b'read\n')
            until(b'READ_OK')
            os.write(fd, b'\x1a')  # Real terminal VSUSP to foreground child.
            # waitpid is the stop acknowledgement; SIGALRM bounds a broken
            # launcher without polling delays or an unbounded cleanup wait.
            def timeout(sig, frame):
                raise TimeoutError('launcher did not stop with child')
            previous_alarm = signal.signal(signal.SIGALRM, timeout)
            signal.alarm(5)
            try:
                got, status = os.waitpid(pid, os.WUNTRACED)
            finally:
                signal.alarm(0)
                signal.signal(signal.SIGALRM, previous_alarm)
            assert got == pid and os.WIFSTOPPED(status), ('launcher did not stop with child', data)
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
            # Close the master before reaping killed children: Darwin can wait
            # for terminal drain during exit while the master remains open.
            os.close(fd)
            if not reaped:
                # Kill both isolated groups; never touch the host terminal group.
                if helper_group is not None and helper_group > 0 and helper_group not in (pid, os.getpgrp()):
                    try: os.killpg(helper_group, signal.SIGKILL)
                    except ProcessLookupError: pass
                try: os.kill(pid, signal.SIGKILL)
                except ProcessLookupError: pass
                try: os.waitpid(pid, 0)
                except ChildProcessError: pass
