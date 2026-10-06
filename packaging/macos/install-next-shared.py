#!/usr/bin/env python3
"""Explicit, Next-only content replacement. Does not manage processes/services."""
import ctypes
import os
from pathlib import Path
import pwd
import shutil
import stat
import sys
import tempfile

from next_shared_bundle import NAME, no_symlinks, require, validate_bundle

RENAME_SWAP = 2


def validate_destination(destination, home):
    destination = no_symlinks(destination)
    home = no_symlinks(home)
    require(destination == home / "Applications" / f"{NAME}.app", "Only ~/Applications/AgentDock Next.app is allowed")
    require(destination.is_dir(), "Replacement requires an existing Next app")
    for directory in (home, destination.parent, destination):
        info = directory.stat()
        require(info.st_uid == os.getuid() and not info.st_mode & (stat.S_IWGRP | stat.S_IWOTH), "Destination must be privately owned")
    return destination


def rename_swap(left, right):
    """Atomically exchange two existing entries on the same filesystem."""
    left = no_symlinks(left)
    right = no_symlinks(right)
    require(left.exists() and right.exists(), "Atomic swap requires two existing entries")
    libc = ctypes.CDLL("/usr/lib/libSystem.B.dylib", use_errno=True)
    swap = libc.renameatx_np
    swap.argtypes = [ctypes.c_int, ctypes.c_char_p, ctypes.c_int, ctypes.c_char_p, ctypes.c_uint]
    swap.restype = ctypes.c_int
    flags = os.O_RDONLY | getattr(os, "O_DIRECTORY", 0)
    left_fd = os.open(left.parent, flags)
    right_fd = os.open(right.parent, flags)
    try:
        if swap(left_fd, os.fsencode(left.name), right_fd, os.fsencode(right.name), RENAME_SWAP):
            raise OSError(ctypes.get_errno(), "Atomic Next content replacement failed")
    finally:
        os.close(left_fd)
        os.close(right_fd)


def main():
    require(sys.platform == "darwin" and len(sys.argv) == 2, "Usage: python3 install-next-shared.py SOURCE.app")
    # Account database, not ambient HOME; callers cannot redirect the install target.
    home = Path(pwd.getpwuid(os.getuid()).pw_dir)
    destination = validate_destination(home / "Applications" / f"{NAME}.app", home)
    source = validate_bundle(sys.argv[1], shared=True)
    require(source != destination and not os.path.samefile(source, destination), "Source must differ from destination")
    validate_bundle(destination)

    # SMAppService tracks the parent application bundle. Replacing the top-level
    # .app directory can make that registration follow the old inode to a backup
    # path. Preserve the installed app root and atomically exchange only Contents.
    original = destination.stat()
    stage = Path(tempfile.mkdtemp(prefix=".agentdock-next-replacement-", dir=destination.parent))
    candidate = stage / f"{NAME}.app"
    candidate.mkdir(mode=0o755)
    candidate_contents = candidate / "Contents"
    destination_contents = destination / "Contents"
    swapped = False

    try:
        shutil.copytree(source / "Contents", candidate_contents)
        validate_bundle(candidate, shared=True)

        validate_destination(destination, home)
        validate_bundle(destination)
        current = destination.stat()
        require((current.st_dev, current.st_ino) == (original.st_dev, original.st_ino), "Destination changed during staging")

        rename_swap(candidate_contents, destination_contents)
        swapped = True

        try:
            current = destination.stat()
            require((current.st_dev, current.st_ino) == (original.st_dev, original.st_ino), "Destination app root changed during replacement")
            validate_bundle(destination, shared=True)
        except Exception:
            # A post-swap validation failure must restore the previously signed
            # Contents before surfacing the error.
            rename_swap(candidate_contents, destination_contents)
            swapped = False
            validate_bundle(destination)
            raise

        print(f"Replaced Contents in {destination}; previous Next Contents retained at {candidate_contents}")
    finally:
        if not swapped:
            shutil.rmtree(stage, ignore_errors=True)


if __name__ == "__main__":
    try:
        main()
    except (ValueError, OSError) as error:
        sys.exit(str(error))
