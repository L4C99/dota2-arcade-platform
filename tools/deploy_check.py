"""Static/isolated Linux deployment checks; never install or start services."""
import json
from pathlib import Path
import shutil
import subprocess
import tempfile

root = Path(__file__).resolve().parents[1]
for path in (root / "configs/examples").glob("*.json.example"):
    json.loads(path.read_text())
with tempfile.TemporaryDirectory(prefix="rc1-units-") as directory:
    temp = Path(directory)
    # Validate systemd syntax using fixture paths/users, not installed units.
    for source in (root / "deploy/systemd").glob("*.service"):
        lines = []
        for line in source.read_text().splitlines():
            if line.startswith(("ExecStart=", "ExecStartPre=")):
                line = line.split("=")[0] + "=/bin/true"
            elif line.startswith(("WorkingDirectory=", "ReadWritePaths=")):
                line = line.split("=")[0] + "=" + str(temp)
            elif line.startswith("EnvironmentFile="):
                line = "EnvironmentFile=-" + str(temp / "absent-fixture.env")
            elif line.startswith(("User=", "Group=")):
                continue
            lines.append(line)
        (temp / source.name).write_text("\n".join(lines) + "\n")
    subprocess.run(["systemd-analyze", "verify", *map(str,temp.glob("*.service"))], check=True)
    for name in ("old", "candidate"):
        release = temp / "releases" / name
        (release / "web").mkdir(parents=True)
        (release / "platform-server").write_text("fixture")
        (release / "platform-server").chmod(0o755)
        (release / "web/index.html").write_text("fixture")
        (release / "PLATFORM-BUILD.json").write_text("{}")
    helper = str(root / "deploy/scripts/switch-platform-release.sh")
    for name in ("old", "candidate", "old"):
        subprocess.run(["bash", helper, str(temp), name], check=True)
        assert (temp / "current").resolve() == temp / "releases" / name
    assert (temp / "previous").resolve() == temp / "releases/candidate"
    assert subprocess.run(["bash", helper, str(temp), "../escape"], capture_output=True).returncode != 0
if shutil.which("caddy"):
    subprocess.run(["caddy", "adapt", "--config", str(root / "deploy/caddy/Caddyfile.example"), "--adapter", "caddyfile"], check=True, stdout=subprocess.DEVNULL)
else:
    print("Caddy binary unavailable: Caddy syntax NOT VERIFIED here")
assert "header_up X-Platform-Client-IP {remote_host}" in (root / "deploy/caddy/Caddyfile.example").read_text()
print("PASS: JSON, systemd syntax with isolated fixture paths, trusted proxy header contract")
