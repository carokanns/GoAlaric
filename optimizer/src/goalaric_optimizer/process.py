from __future__ import annotations

import errno
import os
import signal
import subprocess
import time
from subprocess import Popen


class ProcessGroupError(RuntimeError):
    pass


def process_group_alive(process_group_id: int | None) -> bool:
    if process_group_id is None or process_group_id < 1:
        return False
    if process_group_id == os.getpgrp():
        return False
    try:
        os.killpg(process_group_id, 0)
    except ProcessLookupError:
        return False
    except PermissionError:
        return True
    return True


def _signal_group(process_group_id: int, signum: signal.Signals) -> None:
    if process_group_id < 1 or process_group_id == os.getpgrp():
        raise ProcessGroupError("refusing to signal the current process group")
    try:
        os.killpg(process_group_id, signum)
    except ProcessLookupError:
        return
    except OSError as exc:
        if exc.errno != errno.ESRCH:
            raise ProcessGroupError(f"could not signal process group {process_group_id}: {exc}") from exc


def terminate_process_group(process_group_id: int | None, grace_seconds: float = 1.0) -> None:
    """Terminate a monitor and every child it started in its own process group."""
    if process_group_id is None or process_group_id < 1:
        return
    if process_group_id == os.getpgrp():
        raise ProcessGroupError("refusing to terminate the current process group")
    if not process_group_alive(process_group_id):
        return
    _signal_group(process_group_id, signal.SIGTERM)
    deadline = time.monotonic() + max(0.0, grace_seconds)
    while process_group_alive(process_group_id) and time.monotonic() < deadline:
        time.sleep(0.01)
    if process_group_alive(process_group_id):
        _signal_group(process_group_id, signal.SIGKILL)


def terminate_process(process: Popen[bytes] | Popen[str] | None, grace_seconds: float = 1.0) -> None:
    if process is None:
        return
    try:
        process_group_id = os.getpgid(process.pid)
    except ProcessLookupError:
        process_group_id = process.pid
    terminate_process_group(process_group_id, grace_seconds)
    try:
        process.wait(timeout=max(0.1, grace_seconds + 0.5))
    except subprocess.TimeoutExpired:
        # The process group has already received SIGKILL. This is only a
        # defensive fallback for unusual procfs/reaping delays.
        process.kill()
        process.wait()
