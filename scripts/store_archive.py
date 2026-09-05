#!/usr/bin/env python3
"""Operator-only logical backup/restore. DSNs stay in child environments, not argv/logs.
Restore requires an explicitly confirmed EMPTY disposable/provisioned target database.
The tool never uses --clean or overwrites an existing archive.
"""
from __future__ import annotations
import argparse
import os
from pathlib import Path
import subprocess
import sys
import tempfile


def call(command: list[str], env: dict[str, str], *, output=None) -> bytes:
    try:
        result = subprocess.run(command, env=env, stdout=output or subprocess.PIPE,
                                stderr=subprocess.PIPE, timeout=300, check=False)
    except (OSError, subprocess.TimeoutExpired):
        raise ValueError("database archive command unavailable or timed out") from None
    if result.returncode:
        raise ValueError("database archive command failed; credentials and server messages withheld")
    return result.stdout or b""


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("backup", "restore"))
    parser.add_argument("path", type=Path)
    parser.add_argument("--confirm-empty", action="store_true")
    args = parser.parse_args()
    temporary = None
    try:
        dsn = os.environ.get("CHARTWORKS_STORE_URL", "")
        if not dsn:
            raise ValueError("CHARTWORKS_STORE_URL is required")
        env = dict(os.environ, PGDATABASE=dsn, PGAPPNAME="chartworks-archive")
        if args.action == "backup":
            if args.path.exists() or args.path.is_symlink():
                raise ValueError("archive target already exists")
            fd, temporary = tempfile.mkstemp(prefix=".chartworks-backup-", dir=args.path.parent)
            with os.fdopen(fd, "wb") as output:
                call(["pg_dump", "--format=custom", "--no-owner", "--no-acl"], env, output=output)
                output.flush()
                os.fsync(output.fileno())
            # Atomic publish without overwriting a concurrently created file/symlink.
            os.link(temporary, args.path)
        else:
            if not args.confirm_empty:
                raise ValueError("restore requires --confirm-empty and an empty target database")
            if not args.path.is_file() or args.path.is_symlink():
                raise ValueError("regular archive file required")
            query = "SELECT (SELECT count(*) FROM pg_catalog.pg_namespace WHERE nspname NOT IN ('public','information_schema') AND nspname NOT LIKE 'pg_%') + (SELECT count(*) FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public') + (SELECT count(*) FROM pg_catalog.pg_proc p JOIN pg_catalog.pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname='public');"
            result = call(["psql", "-X", "-tA", "-v", "ON_ERROR_STOP=1", "-c", query], env)
            if result.strip() != b"0":
                raise ValueError("restore target is not empty")
            # Empty conninfo deliberately delegates connection data to PGDATABASE.
            call(["pg_restore", "--dbname=", "--single-transaction", "--exit-on-error",
                  "--no-owner", "--no-acl", str(args.path)], env)
        print("database archive operation completed")
        return 0
    except (OSError, ValueError) as error:
        # OSError filenames and libpq details can contain operator input; do not echo them.
        text = str(error) if isinstance(error, ValueError) else "archive file operation failed"
        print("ERROR: " + text, file=sys.stderr)
        return 1
    finally:
        if temporary:
            try:
                os.unlink(temporary)
            except FileNotFoundError:
                pass


if __name__ == "__main__":
    raise SystemExit(main())
