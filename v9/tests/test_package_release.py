import hashlib
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from zipfile import ZipFile


ROOT = Path(__file__).resolve().parents[2]
COMMON = {
    "agent-team/SKILL.md",
    "agent-team/assets/dashboard.html",
    "agent-team/references/HOSTS.md",
    "agent-team/references/STATE.md",
    "agent-team/references/WORKER_RULES.md",
    "README.md",
    "CUTOVER.md",
}
INSTALLER = {"linux": "install.sh", "windows": "install.ps1", "macos": "install.sh"}


class PackageReleaseTest(unittest.TestCase):
    def test_exact_platform_archives(self):
        for platform, installer in INSTALLER.items():
            with self.subTest(platform=platform), tempfile.TemporaryDirectory() as output:
                subprocess.run(
                    [
                        sys.executable,
                        str(ROOT / "v9/tests/package_release.py"),
                        platform,
                        output,
                    ],
                    check=True,
                )
                archive = Path(output) / f"agent-team-skill-9.0.0-{platform}-any.zip"
                with ZipFile(archive) as bundle:
                    self.assertEqual(set(bundle.namelist()), COMMON | {installer})
                    self.assertIsNone(bundle.testzip())
                    for name in bundle.namelist():
                        self.assertEqual(bundle.read(name), (ROOT / "v9" / name).read_bytes())
                digest, name = Path(str(archive) + ".sha256").read_text().split()
                self.assertEqual(name, archive.name)
                self.assertEqual(digest, hashlib.sha256(archive.read_bytes()).hexdigest())


if __name__ == "__main__":
    unittest.main()
