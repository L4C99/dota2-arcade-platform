"""Build the supported RC packages from a clean checkout; no tag or publication.

Python 3.10+, Go from go.mod, Node 22/npm. Run from any directory.
Output is a NEW directory under ignored dist/ (never overwrite artifacts).
"""
import argparse
import datetime
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tarfile
import zipfile

ROOT = Path(__file__).resolve().parents[1]


def run(args, cwd=ROOT, env=None):
    return subprocess.check_output(args, cwd=cwd, env=env, text=True, encoding="utf-8")


def objects(text):
    decoder = json.JSONDecoder()
    while text.strip():
        item, end = decoder.raw_decode(text.lstrip())
        yield item
        text = text.lstrip()[end:]


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def copy(source, target):
    target.parent.mkdir(parents=True, exist_ok=True)
    shutil.copyfile(source, target)


def licenses(out, modules):
    dest = out / "licenses"
    shutil.copytree(ROOT / "licenses", dest)
    inventory = []
    for name, module in sorted(modules.items()):
        inventory.append({"ecosystem": "go", "name": name, "version": module["Version"]})
        if name.endswith("dota2-arcade-dedicated-core"):
            continue
        source = Path(module["Dir"])
        found = []
        for path in source.rglob("*"):
            if path.is_file() and re.match(r"^(LICENSE|LICENCE|COPYING|NOTICE|PATENTS)(\.|$)", path.name, re.I):
                found.append(path)
                copy(path, dest / "go" / name / path.relative_to(source))
        if not found:
            raise RuntimeError(f"missing license: {name}")
    goroot = Path(run(["go", "env", "GOROOT"]).strip())
    copy(goroot / "LICENSE", dest / "go-runtime-LICENSE.txt")
    # Go's bundled third-party notices, including vendored runtime components.
    for path in (goroot / "src").rglob("*"):
        if path.is_file() and re.match(r"^(LICENSE|NOTICE|PATENTS)(\.|$)", path.name, re.I):
            copy(path, dest / "go-runtime" / path.relative_to(goroot / "src"))
    lock = json.loads((ROOT / "web/package-lock.json").read_text())
    for location, package in sorted(lock["packages"].items()):
        if not location:
            continue
        inventory.append({"ecosystem": "npm", "name": location, "version": package["version"],
                          "license": package.get("license"), "dev": package.get("dev", False)})
        if package.get("dev"):
            continue
        source = ROOT / "web" / location
        found = [p for p in source.iterdir() if p.is_file() and
                 re.search(r"LICENSE|LICENCE|COPYING|NOTICE|AUTHORS", p.name, re.I)]
        if not found:
            raise RuntimeError(f"missing npm license: {location}")
        for path in found:
            relative = location.removeprefix("node_modules/").replace("/node_modules/", "/nested/")
            copy(path, dest / "npm" / relative / path.name)
    (out / "DEPENDENCIES.json").write_text(json.dumps(inventory, indent=2) + "\n")


def archive(source, target, windows=False):
    if windows:
        with zipfile.ZipFile(target, "w", zipfile.ZIP_DEFLATED) as z:
            for path in sorted(source.rglob("*")):
                if path.is_file():
                    z.write(path, path.relative_to(source).as_posix())
    else:
        with tarfile.open(target, "w:gz") as t:
            for path in sorted(source.rglob("*")):
                if path.is_file():
                    info = t.gettarinfo(str(path), path.relative_to(source).as_posix())
                    info.uid = info.gid = 0
                    info.uname = info.gname = ""
                    info.mode = 0o755 if path.name in ("platform-server", "node-controller", "content-tool") or path.suffix == ".sh" else 0o644
                    with path.open("rb") as f:
                        t.addfile(info, f)


def main():
    sys.stdout.reconfigure(encoding="utf-8")
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--version", default="v1.0.0-rc1")
    parser.add_argument("--output", required=True, help="new directory beneath dist/")
    args = parser.parse_args()
    if not re.fullmatch(r"v[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9.-]+)?", args.version):
        raise RuntimeError("invalid version")
    if run(["git", "status", "--porcelain"]).strip():
        raise RuntimeError("release requires a clean tracked and untracked source tree")
    sha = run(["git", "rev-parse", "HEAD"]).strip()
    out = (ROOT / args.output).resolve()
    if not out.is_relative_to((ROOT / "dist").resolve()):
        raise RuntimeError("output must be under ignored dist/")
    out.mkdir(parents=True, exist_ok=False)
    time = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    metadata = {"version": args.version, "gitCommit": sha, "gitDirty": False, "buildTime": time,
                "nodeAPIVersion": 1, "migrationVersion": 19, "d2coreVersion": "0.1.1",
                "d2coreCommit": "988720ad85af1f0d97bfe98ec4da4fcbb070beea"}
    migrations = sorted((ROOT / "internal/platform/store/migrations").glob("*.sql"))
    if [int(p.name.split("_")[0]) for p in migrations] != list(range(1, 20)):
        raise RuntimeError("unexpected migration baseline")
    for path in migrations:
        original = subprocess.check_output(["git", "show", "HEAD:" + path.relative_to(ROOT).as_posix()], cwd=ROOT)
        if path.read_bytes() != original:
            raise RuntimeError("migration checkout bytes differ from Git: " + path.name)
    modules = {}
    artifacts = []
    for goos, commands in [("linux", ["platform-server", "node-controller", "content-tool"]),
                           ("windows", ["node-controller", "content-tool"])]:
        env = dict(os.environ, GOOS=goos, GOARCH="amd64", CGO_ENABLED="0")
        for command in commands:
            name = f"{command}-{goos}-amd64" + (".exe" if goos == "windows" else "")
            flags = f"-X github.com/L4C99/dota2-arcade-platform/internal/buildinfo.Version={args.version} -X github.com/L4C99/dota2-arcade-platform/internal/buildinfo.BuildTime={time}"
            run(["go", "build", "-mod=readonly", "-trimpath", "-buildvcs=true", "-ldflags", flags,
                 "-o", str(out / name), "./cmd/" + command], env=env)
            identity = run(["go", "version", "-m", str(out / name)])
            if "vcs.revision=" + sha not in identity or "vcs.modified=false" not in identity:
                raise RuntimeError("binary source identity mismatch")
            for dep in objects(run(["go", "list", "-mod=readonly", "-deps", "-json", "./cmd/" + command], env=env)):
                module = dep.get("Module", {})
                if module and not module.get("Main"):
                    modules[module["Path"]] = module
            artifacts.append((name, goos + "/amd64"))
    npm = "npm.cmd" if os.name == "nt" else "npm"
    print(run([npm, "ci"], cwd=ROOT / "web"))
    for task in ["lint", "typecheck", "test", "build"]:
        print(run([npm, "run", task], cwd=ROOT / "web"))
    # Web provenance is distributed beside the bundle, without a player UI change.
    (ROOT / "web/dist/BUILD.json").write_text(json.dumps(metadata, indent=2) + "\n")
    common = out / "staging/common"
    common.mkdir(parents=True)
    licenses(common, modules)
    for name in ["LICENSE", "THIRD_PARTY_NOTICES.md"]:
        copy(ROOT / name, common / name)
    (common / "PLATFORM-BUILD.json").write_text(json.dumps(metadata, indent=2) + "\n")
    (common / "MIGRATIONS-SHA256SUMS").write_text("".join(f"{digest(p)}  {p.name}\n" for p in migrations))
    for kind, goos in [("control-plane", "linux"), ("game-node", "linux"), ("game-node", "windows"), ("web", "any")]:
        stage = out / "staging" / (kind + "-" + goos)
        shutil.copytree(common, stage)
        if kind in ("control-plane", "web"):
            shutil.copytree(ROOT / "web/dist", stage / "web")
        if kind != "web":
            # Explicit public-doc/config allowlist; never copy local runtime data.
            for directory in ["docs", "deploy", "configs/examples"]:
                for name in run(["git", "ls-files", directory]).splitlines():
                    if name.endswith((".md", ".service", ".sh", ".ps1", ".example")):
                        copy(ROOT / name, stage / name)
            copy(ROOT / "README.md", stage / "README.md")
            commands = ["platform-server"] if kind == "control-plane" else ["node-controller", "content-tool"]
            for command in commands:
                suffix = ".exe" if goos == "windows" else ""
                copy(out / f"{command}-{goos}-amd64{suffix}", stage / ("bin" if goos == "windows" else "") / (command + suffix))
            if kind == "game-node" and goos == "linux":
                copy(ROOT / "deploy/systemd/start-d2core-manager.sh", stage / "start-d2core-manager.sh")
            if kind == "game-node" and goos == "windows":
                for path in (ROOT / "deploy/windows").glob("*.ps1"):
                    copy(path, stage / "bin" / path.name)
        name = f"dota-arcade-{args.version}-{kind}-{goos}" + ("-amd64.zip" if goos == "windows" else "-amd64.tar.gz" if goos == "linux" else ".tar.gz")
        archive(stage, out / name, goos == "windows")
        artifacts.append((name, goos + ("/amd64" if goos != "any" else "")))
    if run(["git", "status", "--porcelain"]).strip() or run(["git", "rev-parse", "HEAD"]).strip() != sha:
        raise RuntimeError("source changed during build")
    manifest = dict(metadata, artifacts=[dict(filename=name, platform=platform, size=(out / name).stat().st_size,
                    sha256=digest(out / name), sourceSHA=sha) for name, platform in artifacts])
    (out / "MANIFEST.json").write_text(json.dumps(manifest, indent=2) + "\n")
    (out / "SHA256SUMS").write_text("".join(f"{digest(out / name)}  {name}\n" for name, _ in artifacts) + f"{digest(out / 'MANIFEST.json')}  MANIFEST.json\n")
    print(out / "MANIFEST.json")


if __name__ == "__main__":
    main()
