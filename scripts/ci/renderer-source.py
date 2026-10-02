#!/usr/bin/env python3
"""Stage and verify an exact read-only CI checkout without executing its code."""

import hashlib
import os
import pathlib
import stat
import sys


def records(root):
    # Walk through open directory descriptors. A candidate symlink, including a
    # path replaced during traversal, cannot redirect a privileged read.
    flags = os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW
    count, total = 0, 0

    def walk(directory, prefix):
        nonlocal count, total
        for name in sorted(os.listdir(directory)):
            if name in (".", "..") or "/" in name:
                raise ValueError("invalid source path")
            relative = prefix + (name,)
            info = os.stat(name, dir_fd=directory, follow_symlinks=False)
            count += 1
            if count > 100000:
                raise ValueError("source entry bound exceeded")
            if stat.S_ISDIR(info.st_mode):
                child = os.open(name, flags, dir_fd=directory)
                try:
                    actual = os.fstat(child)
                    if (actual.st_dev, actual.st_ino) != (info.st_dev, info.st_ino):
                        raise ValueError("source directory changed during open")
                    yield relative, b"directory", True, b""
                    yield from walk(child, relative)
                finally:
                    os.close(child)
            elif stat.S_ISREG(info.st_mode):
                descriptor = os.open(name, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK, dir_fd=directory)
                with os.fdopen(descriptor, "rb") as file:
                    actual = os.fstat(file.fileno())
                    if not stat.S_ISREG(actual.st_mode) or (actual.st_dev, actual.st_ino) != (info.st_dev, info.st_ino) or actual.st_nlink != 1 or actual.st_size > 32 * 1024 * 1024:
                        raise ValueError("source file type or size refused")
                    payload = file.read(32 * 1024 * 1024 + 1)
                total += len(payload)
                if len(payload) > 32 * 1024 * 1024 or total > 1024 * 1024 * 1024:
                    raise ValueError("source byte bound exceeded")
                yield relative, b"file", bool(actual.st_mode & 0o111), payload
            else:
                raise ValueError("source contains a link or special file")

    descriptor = os.open(root, flags)
    try:
        yield from walk(descriptor, ())
    finally:
        os.close(descriptor)


def snapshot(root):
    digest = hashlib.sha256()
    for relative, kind, executable, payload in records(root):
        for part in ("/".join(relative).encode("utf-8"), kind, b"x" if executable else b"-", payload):
            digest.update(len(part).to_bytes(8, "big"))
            digest.update(part)
    return digest.hexdigest()


def prepare(source, destination, expected_head):
    source, destination = pathlib.Path(source), pathlib.Path(destination)
    if len(expected_head) != 40 or any(c not in "0123456789abcdef" for c in expected_head):
        raise ValueError("invalid source commit")
    head = None
    for relative, _, _, payload in records(source):
        if relative == (".git", "HEAD"):
            head = payload.decode("ascii").strip()
    if head != expected_head:
        raise ValueError("source must be detached at the exact admitted commit")
    if destination.exists() or destination.is_symlink():
        raise ValueError("source destination must not exist")
    before = snapshot(source)
    destination.mkdir(mode=0o755)
    directories = [destination]
    for relative, kind, executable, payload in records(source):
        target = destination.joinpath(*relative)
        if kind == b"directory":
            target.mkdir(mode=0o755)
            directories.append(target)
        else:
            with target.open("xb") as file:
                file.write(payload)
            target.chmod(0o555 if executable else 0o444)
    for directory in reversed(directories):
        directory.chmod(0o555)
    if snapshot(source) != before or snapshot(destination) != before:
        raise ValueError("source changed while staging")
    if any(path.lstat().st_uid != os.geteuid() for path in [destination, *destination.rglob("*")]):
        raise ValueError("staged ownership changed")
    return before


def main():
    if len(sys.argv) == 5 and sys.argv[1] == "prepare":
        print(prepare(*sys.argv[2:]))
    elif len(sys.argv) == 4 and sys.argv[1] == "verify":
        if snapshot(sys.argv[2]) != sys.argv[3]:
            raise ValueError("staged source digest changed")
    else:
        raise ValueError("invalid source staging arguments")


if __name__ == "__main__":
    main()
