#!/usr/bin/env python3
import importlib.util
import json
import tempfile
import unittest
from pathlib import Path

SPEC = importlib.util.spec_from_file_location("release_evidence", Path(__file__).with_name("release-evidence.py"))
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class ReleaseEvidenceTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.root = Path(self.temp.name)
        self.previous_root = MODULE.ROOT
        MODULE.ROOT = self.root
        self.manifest = self.root / "alx.json"
        self.manifest.write_text(json.dumps({"services": [{"id": "napcat-webui"}, {"id": "luckylillia-webui"}]}), encoding="utf-8")

    def tearDown(self):
        MODULE.ROOT = self.previous_root
        self.temp.cleanup()

    def test_manifest_enables_only_evidence_declared_websocket(self):
        path = self.root / "evidence" / "napcat"
        path.mkdir(parents=True)
        path.joinpath("windows-amd64.json").write_text(json.dumps({
            "core": "napcat", "platform": "windows-amd64", "tag": "v1.0.0",
            "asset": "NapCat.Shell.Windows.OneKey.zip", "archiveSha256": "a" * 64,
            "runtimeFingerprint": "b" * 64, "validatedAt": "2026-08-10T00:00:00Z",
            "processModel": "foreground", "websocketRequired": True, "status": "passed",
        }), encoding="utf-8")
        output = self.root / "bundle.json"
        MODULE.prepare_manifest(self.manifest, output, "windows-amd64")
        services = {item["id"]: item for item in json.loads(output.read_text(encoding="utf-8"))["services"]}
        self.assertTrue(services["napcat-webui"]["websocket"])
        self.assertFalse(services["luckylillia-webui"]["websocket"])

    def test_manifest_uses_the_bundle_platform_for_each_core(self):
        path = self.root / "evidence" / "luckylillia"
        path.mkdir(parents=True)
        path.joinpath("darwin-arm64.json").write_text(json.dumps({
            "core": "luckylillia", "platform": "darwin-arm64", "tag": "v1.0.0",
            "asset": "LLBot-CLI-macos-arm64.tar.xz", "archiveSha256": "c" * 64,
            "runtimeFingerprint": "d" * 64, "validatedAt": "2026-08-10T00:00:00Z",
            "processModel": "foreground", "websocketRequired": True, "status": "passed",
        }), encoding="utf-8")
        output = self.root / "bundle.json"
        MODULE.prepare_manifest(self.manifest, output, "darwin-arm64")
        services = {item["id"]: item for item in json.loads(output.read_text(encoding="utf-8"))["services"]}
        self.assertFalse(services["napcat-webui"]["websocket"])
        self.assertTrue(services["luckylillia-webui"]["websocket"])


if __name__ == "__main__":
    unittest.main()
