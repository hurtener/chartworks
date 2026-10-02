import importlib.util
import pathlib
import stat
import tempfile
import unittest
from unittest import mock


spec = importlib.util.spec_from_file_location("renderer_source", pathlib.Path(__file__).with_name("renderer-source.py"))
source = importlib.util.module_from_spec(spec)
spec.loader.exec_module(source)


class RendererSourceTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = pathlib.Path(self.tmp.name)
        self.checkout = self.root / "private-parent" / "candidate"
        self.checkout.mkdir(parents=True)
        self.checkout.parent.chmod(0o700)
        (self.checkout / ".git").mkdir()
        self.head = "a" * 40
        (self.checkout / ".git/HEAD").write_text(self.head + "\n")
        (self.checkout / "main.go").write_text("package main\n")
        (self.checkout / "run.sh").write_text("#!/bin/sh\nexit 99\n")
        (self.checkout / "run.sh").chmod(0o755)
        self.destination = self.root / "job" / "source"
        self.destination.parent.mkdir()

    def make_writable(self):
        if self.destination.exists():
            self.destination.chmod(0o700)
            for path in self.destination.rglob("*"):
                path.chmod(0o700 if path.is_dir() else 0o600)

    def test_exact_staged_bytes_and_readonly_modes(self):
        self.addCleanup(self.make_writable)
        digest = source.prepare(self.checkout, self.destination, self.head)
        self.assertEqual(digest, source.snapshot(self.checkout))
        self.assertEqual(digest, source.snapshot(self.destination))
        for path in [self.destination, *self.destination.rglob("*")]:
            self.assertEqual(path.stat().st_mode & 0o222, 0)
        self.assertEqual(stat.S_IMODE((self.destination / "run.sh").stat().st_mode), 0o555)
        self.assertEqual(stat.S_IMODE((self.destination / "main.go").stat().st_mode), 0o444)
        (self.destination / "main.go").chmod(0o644)
        (self.destination / "main.go").write_text("modified\n")
        self.assertNotEqual(digest, source.snapshot(self.destination))

    def test_wrong_or_symbolic_head_refused(self):
        for value in ("b" * 40, "ref: refs/heads/main"):
            (self.checkout / ".git/HEAD").write_text(value + "\n")
            with self.assertRaises(ValueError):
                source.prepare(self.checkout, self.destination, self.head)
            self.assertFalse(self.destination.exists())

    def test_links_and_existing_destination_refused(self):
        (self.checkout / "escape").symlink_to(self.root)
        with self.assertRaises(ValueError):
            source.prepare(self.checkout, self.destination, self.head)
        (self.checkout / "escape").unlink()
        self.destination.mkdir()
        with self.assertRaises(ValueError):
            source.prepare(self.checkout, self.destination, self.head)

    def test_link_replacement_cannot_redirect_open(self):
        outside = self.root / "outside"
        outside.write_text("outside-source-canary")
        original_open = os_open = source.os.open

        def replaced_open(path, flags, *args, **kwargs):
            if path == "main.go" and "dir_fd" in kwargs:
                (self.checkout / "main.go").unlink()
                (self.checkout / "main.go").symlink_to(outside)
            return original_open(path, flags, *args, **kwargs)

        with mock.patch.object(source.os, "open", replaced_open):
            with self.assertRaises(OSError):
                source.prepare(self.checkout, self.destination, self.head)
        self.assertIs(source.os.open, os_open)
        self.assertFalse(self.destination.exists())

    def test_fifo_replacement_cannot_block_open(self):
        original_open = source.os.open

        def replaced_open(path, flags, *args, **kwargs):
            if path == "main.go" and "dir_fd" in kwargs:
                (self.checkout / "main.go").unlink()
                source.os.mkfifo(self.checkout / "main.go")
            return original_open(path, flags, *args, **kwargs)

        with mock.patch.object(source.os, "open", replaced_open):
            with self.assertRaises(ValueError):
                source.prepare(self.checkout, self.destination, self.head)
        self.assertFalse(self.destination.exists())


if __name__ == "__main__":
    unittest.main()
