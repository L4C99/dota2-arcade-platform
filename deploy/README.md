# Reference deployment assets

These files describe an Ubuntu 24.04 LTS amd64 control plane and native Linux/Windows game nodes. They are examples; choose private host paths and accounts before installation. Production deployment requires separate authorization.

Use the [RC1 packages and identity procedure](../docs/release.md). Run `version`
on the unpacked executables, compare PLATFORM-BUILD.json and SHA256SUMS, and keep
the included licenses. Node packages deliberately exclude d2core server binaries;
retrieve and verify official fixed v0.1.1 separately. Its BUILD.json is required.

## Linux Web control plane

1. Install PostgreSQL and Caddy from trusted packages. Provision a dedicated database, role and persistent data volume. Store its URL in `/etc/dota-arcade/platform.env` with mode 0600. Set `PLATFORM_ENV=production`, `PLATFORM_PUBLIC_ORIGIN=https://...`, `PLATFORM_LISTEN_ADDR=127.0.0.1:8080`, explicit `PLATFORM_MAX_PARTY_SIZE`, and `PLATFORM_WEB_ROOT=/opt/dota-arcade/current/web`.
2. Build `platform-server` for Linux amd64 and `web/dist` in CI or a build machine. Package them under a versioned directory `/opt/dota-arcade/releases/<build>/`; link `current` to that directory. The source checkout is never the runtime directory.
3. Install `systemd/platform-server.service` and adapt `caddy/Caddyfile.example` to the real public domain. Caddy owns TLS; Platform listens only on loopback. The platform unit runs `migrate` before `serve`, so a failed migration prevents startup.
4. Configure a private libpq service file for the same database and set `PGSERVICE` (and `PGSERVICEFILE` if needed), so the backup command does not put a credential-bearing URL in process arguments. Back up before every update with `scripts/backup-postgres.sh` and verify the dump. See [operations](../docs/operations.md) for recovery and rollback.
5. Verify `systemctl status`, the local `/healthz`, public HTTPS `/healthz`, player page and admin login. Do not expose PostgreSQL or Platform loopback HTTP directly to the internet.

Provision the ordinary `arcade-platform` account and group before installing the
unit. Create `/var/lib/dota-arcade` owned by it (0750), root-owned release
directories under `/opt/dota-arcade/releases` (0755 directories; 0755 binary and
0644 Web files), and root-owned `/etc/dota-arcade` (0700). systemd reads the
root-owned 0600 environment file; do not make it publicly readable or copy it into
a release. PostgreSQL and Caddy use their own accounts/persistent directories.
Backups belong in a protected persistent directory (0700), with private libpq
service/password files (0600) owned by the backup user. The backup helper rejects
publicly readable or symlinked service files, uses a unique temporary dump, checks
the custom-format TOC, and publishes without overwriting an existing backup.
It removes partial output on failure. `pg_restore --list` is not a restore test.

Updates follow **backup + separate restore verification → package validation →
stop writes → candidate migrate → switch Platform/Web together → restart → local
health → public HTTPS check**. Retain the previous immutable release. An explicit
pointer helper is provided as `scripts/switch-platform-release.sh ABS_ROOT NAME`;
it updates only `current` and records the old target in `previous`. Run migrate
using the private environment before the switch, then `systemctl restart
platform-server`. It does not inject credentials, migrate, restart or assert health.
For compatible code rollback, record the `previous` release name and use the same
helper, then restart and check health. For schema-incompatible rollback, stop
writes, restore a verified pre-upgrade backup into a recovery DB, reconcile
Node/Controller/d2core side effects, and only then use the old binary with that
matching DB. Restoring PostgreSQL does not stop Dota.

Caddy must overwrite `X-Platform-Client-IP` with `{remote_host}` as in the example;
untrusted X-Forwarded-For is not the login-throttle identity. Keep upstream
loopback-only and do not insert an untrusted local proxy. Fresh Ubuntu install,
production TLS and production restore remain later real environment gates.

## Linux game node

Install Bash and `jq` first (Ubuntu: `sudo apt-get install jq`); the manager wrapper checks `jq` before startup. Install `node-controller`, `content-tool`, official fixed d2core v0.1.1 and `BUILD.json` in a versioned native directory, with `current` pointing at the active build. Keep the Controller JSON and node Secret outside that directory under `/etc/dota-arcade-node/`; allow the shared ordinary runtime account to read the Secret. Keep d2core data, logs, templates and ContentRoot on persistent storage. Adjust `configs/examples/node-controller.linux.json.example` for actual local paths and network facts. Install the three Linux files in `systemd/` and the executable `start-d2core-manager.sh` into the active node build.

`start-d2core-manager.sh` reads the Controller JSON with `jq` and passes its `network.localPortMin`/`localPortMax` to fixed d2core `serve`. This makes both components use the same local game-port bounds. The Controller service follows the manager and retries on failure. Neither unit configures firewall, NAT, Steam or Dota.

Provision the same ordinary `arcade-node` user/group for Controller, d2core and
Dota. Use persistent `/var/lib/dota-arcade-node` directories owned by that account
(0750), private config directory root:arcade-node (0750), and config/Secret files
root:arcade-node (0640) or account-owned 0600. Keep logs there, not inside immutable
release directories. Templates, d2core data-dir and all critical paths must be
ASCII absolute paths; data/cfg must not sit below content symlinks. Populate the
actual template bindings, ContentRoot metadata and Dota paths before admission.
Run `node-controller check --config ABS_CONFIG`, verify d2core BUILD.json, jq and
the shared port range, start manager first and Controller next, then require
compatible heartbeat/readback/reconcile before Resume. Updates and rollback use
the Drain/occupied=0/open jobs=0/list-empty runbook in operations.md.

## Windows game node

Place `node-controller.exe`, `content-tool.exe`, official d2core v0.1.1 `d2core.exe` and its `BUILD.json` under `C:\ProgramData\DotaArcadeNode\bin`, with JSON and Secret under `config`, plus persistent `data`, `content`, `templates` and `logs`. Copy the three scripts in `windows/` to `bin`. Edit `configs/examples/node-controller.windows.json.example` and validate all local paths and network mappings.

Run `install-node.ps1 -Root C:\ProgramData\DotaArcadeNode -ValidateOnly` first. After checking the output and runtime account, run it without `-ValidateOnly` in an elevated PowerShell session and supply that account's credential at the prompt. It registers two startup scheduled tasks under the same account. Start the d2core task first, then Controller, and verify task results, transcripts, Platform heartbeat, direct d2core list and Dota process state. The d2core wrapper reads the exact same Controller JSON port bounds on every start. The installer does not alter firewall, NAT, Dota files or router state.

On both systems, `content-tool` uses `CONTENT_ROOT` and `DOTA_ROOT` or equivalent `--content-root`/`--dota-root` flags. Set each Controller content binding's `metadataPath` to `<ContentRoot>/<WorkshopID>/metadata/current.json` and `currentLinkPath` to `<DotaRoot>/game/dota_addons/<WorkshopID>`. The Tool never runs automatically.

On Windows restrict config, Secret and persistent directories with NTFS ACLs to
the ordinary runtime account and authorized administrators; do not grant broad
Users read access to the Secret. Preserve the prior binaries outside the active
bin directory for rollback. `ValidateOnly` is an inventory/fixed-core preflight;
also run `node-controller.exe check --config ABS_CONFIG` for the full config and
Secret-format check. Scheduled tasks may trigger concurrently at boot; Controller
retries until manager is reachable, and operator startup is manager then
Controller. ValidateOnly does not register tasks. Unattended install/reboot and
Windows public entry remain **NOT VERIFIED**, not unsupported.
