import hashlib
import importlib.util
import json
import sys
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]
SPIKE = ROOT / "tools/psalm-spike"
SPEC = importlib.util.spec_from_file_location("psalm_native_config", SPIKE / "native_config.py")
NATIVE = importlib.util.module_from_spec(SPEC)
with patch.object(sys, "path", [str(SPIKE), *sys.path]):
    SPEC.loader.exec_module(NATIVE)


class NativePsalmConfigTest(unittest.TestCase):
    def test_runtime_probe_checks_actual_patch_minor_and_extensions(self):
        value = {
            "version": "8.4.26",
            "id": 80426,
            "runtime": "8.4",
            "extensions": ["tokenizer", "dom"],
        }
        result = SimpleNamespace(returncode=0, stderr="", stdout=json.dumps(value))
        measured = NATIVE.runtime_details(result, "8.4")
        self.assertEqual(measured["version"], "8.4.26")
        self.assertEqual(measured["extensions"], ["dom", "tokenizer"])
        with self.assertRaisesRegex(ValueError, "no fallback"):
            NATIVE.runtime_details(result, "8.3")
        for changes in (
            {"version": "8.4.26-dev"},
            {"id": 80336},
            {"id": True},
            {"runtime": "8.3"},
            {"extensions": []},
            {"extensions": ["dom", "dom"]},
            {"extensions": [None]},
        ):
            with self.subTest(changes=changes), self.assertRaises(ValueError):
                result.stdout = json.dumps(value | changes)
                NATIVE.runtime_details(result)
        result.returncode = 1
        with self.assertRaisesRegex(ValueError, "probe failed"):
            NATIVE.runtime_details(result)
        result.returncode, result.stderr = 0, "startup warning"
        with self.assertRaisesRegex(ValueError, "probe failed"):
            NATIVE.runtime_details(result)
        result.stderr, result.stdout = "", "[]"
        with self.assertRaisesRegex(ValueError, "object"):
            NATIVE.runtime_details(result)

    def test_php84_cases_include_unsafe_and_escaped_property_hooks(self):
        for kind in ("php84-hook", "php84-hook-safe"):
            with self.subTest(kind=kind), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                NATIVE.seed(root, kind, "", "")
                code = (root / "src/HookedInput.php").read_text()
                self.assertIn("get =>", code)
                self.assertIn("$_GET", code)
                self.assertEqual("htmlspecialchars" in code, kind.endswith("-safe"))

    def test_bundle_uses_verified_inventory_and_its_own_schema(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            for name in ("autoload.php", "bin/psalm", "vimeo/psalm/config.xsd"):
                path = root / "vendor" / name
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_text("fixture")
            args = SimpleNamespace(bundle=root, phar=None, schema=None)
            with patch.object(
                NATIVE, "installed_packages", return_value=(b"lock", {"psalm": {}})
            ) as check:
                executable, identity = NATIVE.engine(args)
            check.assert_called_once_with(root)
            self.assertEqual(executable, "/schema/bin/psalm")
            self.assertEqual(identity["bundle_lock_sha256"], hashlib.sha256(b"lock").hexdigest())
            self.assertEqual(identity["engine"], "composer")
            args.schema = root / "override.xsd"
            with self.assertRaisesRegex(ValueError, "own pinned schema"):
                NATIVE.engine(args)
            args.schema = None
            with patch.object(NATIVE, "installed_packages", side_effect=ValueError("stale bundle")):
                with self.assertRaisesRegex(ValueError, "stale bundle"):
                    NATIVE.engine(args)

    def test_docker_mounts_bundle_without_phar_and_keeps_resource_limits(self):
        args = SimpleNamespace(
            bundle=Path("/bundle"),
            phar=None,
            schema=None,
            binary=Path("/helper"),
            image="fixture-image",
        )
        command = NATIVE.docker(args, Path("/project"), Path("/private"), "php")
        self.assertIn("type=bind,src=/bundle/vendor,dst=/schema,readonly", command)
        self.assertFalse(any("psalm.phar" in value for value in command))
        self.assertIn("--cpus=0.2", command)
        self.assertIn("--network=none", command)
        self.assertIn("--read-only", command)
        self.assertIn("--pull=never", command)

    def test_phar_still_requires_schema_and_the_pinned_checksum(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            args = SimpleNamespace(bundle=None, phar=root / "psalm.phar", schema=None)
            args.phar.write_bytes(b"wrong artifact")
            with self.assertRaisesRegex(ValueError, "requires --schema"):
                NATIVE.engine(args)
            args.schema = root / "config.xsd"
            args.schema.write_text("schema")
            with self.assertRaisesRegex(ValueError, "checksum mismatch"):
                NATIVE.engine(args)
            with patch.object(NATIVE, "SHA256", hashlib.sha256(args.phar.read_bytes()).hexdigest()):
                executable, identity = NATIVE.engine(args)
            self.assertEqual(executable, "/opt/psalm.phar")
            self.assertEqual(identity["engine"], "phar")

    def test_bundle_snapshot_detects_code_changes_and_refuses_symlinks(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source = root / "source.php"
            source.write_text("before")
            before = NATIVE.bundle_snapshot(root)
            summary = {"passed": False}
            NATIVE.verify_bundle(root, before, summary)
            self.assertTrue(summary["bundle_preserved"])
            self.assertFalse(summary["passed"])
            source.write_text("after!")
            self.assertNotEqual(NATIVE.bundle_snapshot(root), before)
            summary["passed"] = True
            with self.assertRaisesRegex(RuntimeError, "inputs changed"):
                NATIVE.verify_bundle(root, before, summary)
            self.assertEqual(summary, {"passed": False, "bundle_preserved": False})
            source.unlink()
            source.symlink_to(ROOT / "go.mod")
            with self.assertRaises(ValueError):
                NATIVE.bundle_snapshot(root)
            summary["passed"] = True
            with self.assertRaises(ValueError):
                NATIVE.verify_bundle(root, before, summary)
            self.assertEqual(summary, {"passed": False, "bundle_preserved": False})
            source.unlink()
            (root / "linked-directory").symlink_to(ROOT / "internal", target_is_directory=True)
            with self.assertRaises(ValueError):
                NATIVE.bundle_snapshot(root)


if __name__ == "__main__":
    unittest.main()
