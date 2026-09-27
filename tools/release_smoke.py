"""Validate archive integrity and smoke the native executables without Dota."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import struct
import subprocess
import tarfile
import tempfile
import time
import urllib.request
import zipfile


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("directory", type=Path)
    args = parser.parse_args()
    root = args.directory.resolve()
    manifest = json.loads((root / "MANIFEST.json").read_text())
    assert manifest["gitDirty"] is False and manifest["migrationVersion"] == 19
    for line in (root / "SHA256SUMS").read_text().splitlines():
        expected, name = line.split("  ", 1)
        assert Path(name).name == name
        assert hashlib.sha256((root / name).read_bytes()).hexdigest() == expected
    for item in manifest["artifacts"]:
        path = root / item["filename"]
        assert path.stat().st_size == item["size"]
        assert hashlib.sha256(path.read_bytes()).hexdigest() == item["sha256"]
        assert item["sourceSHA"] == manifest["gitCommit"]
        if path.name.endswith((".zip", ".tar.gz")):
            if path.suffix == ".zip":
                with zipfile.ZipFile(path) as archive:
                    assert archive.testzip() is None
                    names = archive.namelist()
                    identity = json.loads(archive.read("PLATFORM-BUILD.json"))
                    if "game-node" in path.name:
                        assert {"bin/node-controller.exe", "bin/content-tool.exe"}.issubset(names)
            else:
                with tarfile.open(path) as archive:
                    names = archive.getnames()
                    identity = json.load(archive.extractfile("PLATFORM-BUILD.json"))
                    for member in archive.getmembers():
                        assert member.isfile()
                        if member.name in ("platform-server", "node-controller", "content-tool") or member.name.endswith(".sh"):
                            assert member.mode == 0o755
                    if "game-node" in path.name:
                        assert {"node-controller", "content-tool"}.issubset(names)
            assert identity["gitCommit"] == manifest["gitCommit"] and identity["gitDirty"] is False
            assert {"LICENSE", "THIRD_PARTY_NOTICES.md", "licenses/d2core-LICENSE.txt", "DEPENDENCIES.json"}.issubset(names)
            for name in names:
                assert not name.startswith("/") and ".." not in Path(name).parts
                assert not any(part in (".local", ".env", "node_modules", ".git") for part in Path(name).parts)
                assert not name.endswith((".vpk", ".dump", ".backup", ".key", ".pem", ".log"))
                assert Path(name).name not in ("d2core", "d2core.exe", "node.secret")
        else:
            data = path.read_bytes()
            if item["platform"] == "linux/amd64":
                assert data[:5] == b"\x7fELF\x02" and struct.unpack_from("<H", data, 18)[0] == 62
            else:
                offset = struct.unpack_from("<I", data, 0x3C)[0]
                assert data[:2] == b"MZ" and data[offset:offset+4] == b"PE\0\0"
                assert struct.unpack_from("<H", data, offset+4)[0] == 0x8664
    native = "windows" if os.name == "nt" else "linux"
    suffix = ".exe" if os.name == "nt" else ""
    def binary(command):
        path = root / f"{command}-{native}-amd64{suffix}"
        if os.name != "nt":
            path.chmod(0o755)
        return str(path)
    def call(command, *arguments, success=True):
        p = subprocess.run([binary(command), *map(str, arguments)], capture_output=True, text=True)
        assert (p.returncode == 0) == success, (command, arguments, p.stderr)
        return p.stdout
    commands = ["node-controller", "content-tool"] + (["platform-server"] if native == "linux" else [])
    for command in commands:
        version = json.loads(call(command, "version"))
        for key in ("version", "gitCommit", "gitDirty", "buildTime"):
            assert version[key] == manifest[key], (command, key)
        assert version["os"] == native and version["arch"] == "amd64"
        call(command, "--help")
    with tempfile.TemporaryDirectory(prefix="rc1-smoke-") as directory:
        temp = Path(directory)
        content, dota = temp / "content", temp / "dota"
        content.mkdir(); dota.mkdir()
        flags = ("--content-root", content, "--dota-root", dota)
        call("content-tool", *flags, "status", "123")
        call("content-tool", "--content-root", "relative", "--dota-root", dota, "status", "123", success=False)
        source = temp / "synthetic-input"
        source.write_bytes(b"RC1 isolated filesystem fixture, not game assets")
        call("content-tool", *flags, "prepare", "123", "rc1-fixture", source)
        state = json.loads(call("content-tool", *flags, "status", "123"))
        assert "rc1-fixture" in state["preparedVersions"]
        secret = temp / "secret"
        secret.write_text("A" * 43)
        config = {"platformUrl":"https://platform.example.org", "nodeId":"00000000-0000-4000-8000-000000000001",
                  "nodeSecretFile":str(secret), "d2coreBuildFile":str(temp / "BUILD.json"),
                  "d2coreDataDir":str(temp / "data"), "hardMaxInstances":1,
                  "network":{"connectHost":"node.example.org", "localPortMin":28000,"localPortMax":28001,"mappingMode":"identity"},
                  "templateBindings":{},"contentBindings":[]}
        configPath = temp / "node-controller.json"
        configPath.write_text(json.dumps(config))
        call("node-controller", "check", "--config", configPath)
        config["hardMaxInstances"] = 99
        configPath.write_text(json.dumps(config))
        call("node-controller", "check", "--config", configPath, success=False)
    if native == "linux" and os.environ.get("PLATFORM_SMOKE_DATABASE_URL"):
        env = dict(os.environ, PLATFORM_DATABASE_URL=os.environ["PLATFORM_SMOKE_DATABASE_URL"],
                   PLATFORM_ENV="development", PLATFORM_MAX_PARTY_SIZE="4",
                   PLATFORM_LISTEN_ADDR="127.0.0.1:18089", PLATFORM_PUBLIC_ORIGIN="http://127.0.0.1:18089",
                   PLATFORM_WEB_ROOT=str(root / "staging/control-plane-linux/web"))
        subprocess.run([binary("platform-server"), "migrate"], env=env, check=True, capture_output=True)
        process = subprocess.Popen([binary("platform-server"), "serve"], env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        try:
            for attempt in range(100):
                try:
                    with urllib.request.urlopen("http://127.0.0.1:18089/healthz", timeout=1) as response:
                        assert response.status == 200
                    break
                except OSError:
                    if process.poll() is not None: raise RuntimeError("Platform exited before health")
                    time.sleep(0.1)
            else: raise RuntimeError("Platform health timed out")
            for route in ("/", "/admin", "/BUILD.json"):
                with urllib.request.urlopen("http://127.0.0.1:18089" + route, timeout=2) as response:
                    assert response.status == 200
                    if route == "/BUILD.json": assert json.load(response)["gitCommit"] == manifest["gitCommit"]
        finally:
            process.terminate()
            try: process.wait(timeout=10)
            except subprocess.TimeoutExpired:
                process.kill(); process.wait()
        print("PASS: packaged Platform migrate/health and paired Web in disposable DB setup")
    print(f"PASS: checksums, architecture, archives, permissions, notices, {native} artifact identity/config/filesystem smoke")


if __name__ == "__main__":
    main()
