#!/usr/bin/env python3
"""Explicit, Next-only atomic replacement. Does not manage processes/services."""
import ctypes
import os
from pathlib import Path
import pwd
import shutil
import stat
import sys
import tempfile

from next_shared_bundle import NAME, no_symlinks, require, validate_bundle


def validate_destination(destination, home):
    destination = no_symlinks(destination)
    home = no_symlinks(home)
    require(destination == home / "Applications" / f"{NAME}.app", "Only ~/Applications/AgentDock Next.app is allowed")
    require(destination.is_dir(), "Replacement requires an existing Next app")
    for directory in (home, destination.parent, destination):
        info = directory.stat()
        require(info.st_uid == os.getuid() and not info.st_mode & (stat.S_IWGRP | stat.S_IWOTH), "Destination must be privately owned")
    return destination


def main():
    require(sys.platform == "darwin" and len(sys.argv) == 2, "Usage: python3 install-next-shared.py SOURCE.app")
    # Account database, not ambient HOME; callers cannot redirect the install target.
    home = Path(pwd.getpwuid(os.getuid()).pw_dir)
    destination = validate_destination(home / "Applications" / f"{NAME}.app", home)
    source = validate_bundle(sys.argv[1], shared=True)
    require(source != destination, "Source must differ from destination")
    validate_bundle(destination)
    original = destination.stat()
    stage = Path(tempfile.mkdtemp(prefix=".agentdock-next-replacement-", dir=destination.parent))
    candidate = stage / f"{NAME}.app"
    swapped = False
    try:
        shutil.copytree(source, candidate)
        validate_bundle(candidate, shared=True)
        validate_destination(destination, home)
        validate_bundle(destination)
        current = destination.stat()
        require((current.st_dev, current.st_ino) == (original.st_dev, original.st_ino), "Destination changed during staging")
        libc = ctypes.CDLL("/usr/lib/libSystem.B.dylib", use_errno=True)
        swap = libc.renameatx_np
        swap.argtypes = [ctypes.c_int, ctypes.c_char_p, ctypes.c_int, ctypes.c_char_p, ctypes.c_uint]
        swap.restype = ctypes.c_int
        # RENAME_SWAP atomically exchanges two existing entries on the same volume.
        parent_fd = os.open(destination.parent, os.O_RDONLY)
        try:
            if swap(parent_fd, os.fsencode(candidate.relative_to(destination.parent)), parent_fd, os.fsencode(destination.name), 2):
                raise OSError(ctypes.get_errno(), "Atomic Next replacement failed")
        finally:
            os.close(parent_fd)
        swapped = True
        print(f"Replaced {destination}; previous Next bundle retained at {candidate}")
    finally:
        if not swapped:
            shutil.rmtree(stage)


if __name__ == "__main__":
    try:
        main()
    except (ValueError, OSError) as error:
        sys.exit(str(error))
