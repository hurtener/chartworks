import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

import store_archive


class ArchiveTests(unittest.TestCase):
    def test_uri_is_expanded_without_argv_credentials(self):
        with patch.dict(os.environ, {"PGHOST": "wrong", "PGOPTIONS": "wrong"}):
            env = store_archive.connection_environment("postgres://user:p%40ss@localhost:5434/database?sslmode=disable")
        self.assertEqual(env["PGPASSWORD"], "p@ss")
        self.assertEqual(env["PGHOST"], "localhost")
        self.assertEqual(env["PGDATABASE"], "database")
        self.assertNotIn("PGOPTIONS", env)

    def test_bad_or_ambiguous_uri_is_safe(self):
        for uri in ("", "secret-canary", "postgres://host/db", "postgres://u:p@host/db?sslmode=require&sslmode=disable", "postgres://u:p@host/db?unknown=1", "postgres://u:p@host/db?connect_timeout=0", "postgres://u:p@host:99999/db"):
            with self.subTest(uri=uri), self.assertRaisesRegex(ValueError, "explicit PostgreSQL URI"):
                store_archive.connection_environment(uri)

    def test_provider_stderr_never_leaks(self):
        with patch("subprocess.run", return_value=subprocess.CompletedProcess([], 1, b"", b"password-secret")):
            with self.assertRaisesRegex(ValueError, "messages withheld"):
                store_archive.call(["psql"], {})

    def test_timeout_and_missing_tool(self):
        for error in (OSError("secret"), subprocess.TimeoutExpired("command", 1)):
            with patch("subprocess.run", side_effect=error), self.assertRaisesRegex(ValueError, "unavailable or timed out"):
                store_archive.call(["psql"], {})

    def test_backup_does_not_replace_existing_path(self):
        with tempfile.TemporaryDirectory() as d:
            path = Path(d) / "backup"
            path.write_text("existing")
            with patch.dict(os.environ, {"CHARTWORKS_STORE_URL": "postgres://u:p@host/db"}), patch("sys.argv", ["archive", "backup", str(path)]), patch("builtins.print"):
                self.assertEqual(store_archive.main(), 1)
            self.assertEqual(path.read_text(), "existing")

    def test_restore_requires_confirmation(self):
        with patch.dict(os.environ, {"CHARTWORKS_STORE_URL": "postgres://u:p@host/db"}), patch("sys.argv", ["archive", "restore", "unused"]), patch("builtins.print"):
            self.assertEqual(store_archive.main(), 1)
