#!/usr/bin/env python3
"""Operator-only logical backup/restore; credentials never enter argv or logs.

Requires a PostgreSQL URI and matching PostgreSQL client tools. Restore requires
an explicitly confirmed EMPTY database controlled exclusively by the operator.
Only restore trusted archives: PostgreSQL archives can contain executable SQL.
"""
from __future__ import annotations

import argparse
import os
from pathlib import Path
import subprocess
import sys
import tempfile
from urllib.parse import parse_qsl, unquote, urlsplit


def connection_environment(dsn: str) -> dict[str, str]:
    """libpq does not expand a URI supplied through PGDATABASE; use explicit fields."""
    try:
        u = urlsplit(dsn)
        if u.scheme not in ("postgres", "postgresql") or not u.hostname or not u.username or not u.path[1:] or u.fragment:
            raise ValueError
        port = u.port or 5432
        if not 1 <= port <= 65535:
            raise ValueError
        env = {k: v for k, v in os.environ.items() if not k.startswith("PG") and k != "CHARTWORKS_STORE_URL"}
        env.update(PGHOST=u.hostname, PGPORT=str(port), PGUSER=unquote(u.username),
                   PGPASSWORD=unquote(u.password or ""), PGDATABASE=unquote(u.path[1:]),
                   PGAPPNAME="chartworks-archive", PGCONNECT_TIMEOUT="5")
        allowed = {"sslmode": "PGSSLMODE", "sslrootcert": "PGSSLROOTCERT",
                   "sslcert": "PGSSLCERT", "sslkey": "PGSSLKEY",
                   "connect_timeout": "PGCONNECT_TIMEOUT", "target_session_attrs": "PGTARGETSESSIONATTRS"}
        seen = set()
        for key, value in parse_qsl(u.query, keep_blank_values=True, strict_parsing=True):
            if key not in allowed or key in seen or not value:
                raise ValueError
            seen.add(key)
            env[allowed[key]] = value
        if not 1 <= int(env["PGCONNECT_TIMEOUT"]) <= 60:
            raise ValueError
        if any("\x00" in value or "\n" in value or "\r" in value for key, value in env.items() if key.startswith("PG")):
            raise ValueError
        return env
    except (ValueError, TypeError):
        raise ValueError("explicit PostgreSQL URI with supported connection options required") from None


def call(command: list[str], env: dict[str, str], *, output=None) -> bytes:
    try:
        result = subprocess.run(command, env=env, stdin=subprocess.DEVNULL,
                                stdout=output if output is not None else subprocess.PIPE,
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
        env = connection_environment(os.environ.get("CHARTWORKS_STORE_URL", ""))
        if args.action == "backup":
            if args.path.exists() or args.path.is_symlink():
                raise ValueError("archive target already exists")
            fd, temporary = tempfile.mkstemp(prefix=".chartworks-backup-", dir=args.path.parent)
            with os.fdopen(fd, "wb") as output:
                call(["pg_dump", "--no-password", "--format=custom", "--no-owner", "--no-acl"], env, output=output)
                output.flush()
                os.fsync(output.fileno())
            os.link(temporary, args.path)
        else:
            if not args.confirm_empty:
                raise ValueError("restore requires --confirm-empty and an empty target database")
            if not args.path.is_file() or args.path.is_symlink():
                raise ValueError("regular trusted archive file required")
            query = "SELECT (SELECT count(*) FROM pg_catalog.pg_namespace WHERE nspname NOT IN ('public','information_schema') AND nspname NOT LIKE 'pg_%') + (SELECT count(*) FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public') + (SELECT count(*) FROM pg_catalog.pg_proc p JOIN pg_catalog.pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname='public');"
            result = call(["psql", "--no-password", "-X", "-tA", "-v", "ON_ERROR_STOP=1", "-c", query], env)
            if result.strip() != b"0":
                raise ValueError("restore target is not empty")
            call(["pg_restore", "--no-password", "--dbname=", "--single-transaction", "--exit-on-error",
                  "--no-owner", "--no-acl", str(args.path.resolve())], env)
        print("database archive operation completed")
        return 0
    except (OSError, ValueError) as error:
        text = str(error) if isinstance(error, ValueError) else "archive file operation failed"
        print("ERROR: " + text, file=sys.stderr)
        return 1
    finally:
        if temporary:
            try:
                os.unlink(temporary)
            except OSError:
                pass  # A private incomplete file is safe; never replace the original failure with a path leak.


if __name__ == "__main__":
    raise SystemExit(main())
