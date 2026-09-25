import argparse
from hashlib import sha256
from pathlib import Path
from zipfile import ZIP_DEFLATED, ZipFile


ROOT = Path(__file__).resolve().parents[1]
SKILL_FILES = {
    "agent-team/SKILL.md",
    "agent-team/assets/dashboard.html",
    "agent-team/references/HOSTS.md",
    "agent-team/references/STATE.md",
    "agent-team/references/WORKER_RULES.md",
}
INSTALLER = {"linux": "install.sh", "windows": "install.ps1", "macos": "install.sh"}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("platform", choices=sorted(INSTALLER))
    parser.add_argument("output_dir", type=Path)
    args = parser.parse_args()

    tree = ROOT / "agent-team"
    entries = list(tree.rglob("*"))
    if tree.is_symlink() or any(path.is_symlink() for path in entries):
        raise SystemExit("skill tree contains a symlink")
    actual = {path.relative_to(ROOT).as_posix() for path in entries if path.is_file()}
    if actual != SKILL_FILES:
        raise SystemExit("skill tree differs from exact package allowlist")

    names = sorted(SKILL_FILES | {"README.md", "CUTOVER.md", INSTALLER[args.platform]})
    for name in names:
        source = ROOT / name
        if not source.is_file() or source.is_symlink():
            raise SystemExit(f"unsafe or missing package file: {name}")

    args.output_dir.mkdir(parents=True, exist_ok=True)
    archive = args.output_dir / f"agent-team-skill-9.0.0-{args.platform}-any.zip"
    with ZipFile(archive, "w", ZIP_DEFLATED) as bundle:
        for name in names:
            bundle.write(ROOT / name, arcname=name)
    with ZipFile(archive) as bundle:
        if set(bundle.namelist()) != set(names) or bundle.testzip() is not None:
            raise SystemExit("archive member or CRC mismatch")
    Path(str(archive) + ".sha256").write_text(
        f"{sha256(archive.read_bytes()).hexdigest()}  {archive.name}\n"
    )


if __name__ == "__main__":
    main()
