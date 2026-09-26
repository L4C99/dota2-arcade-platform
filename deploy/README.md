# Reference deployment assets

These files describe an Ubuntu 24.04 LTS amd64 control plane and native Linux/Windows game nodes. They are examples; choose private host paths and accounts before installation. Production deployment requires separate authorization.

## Linux Web control plane

1. Install PostgreSQL and Caddy from trusted packages. Provision a dedicated database, role and persistent data volume. Store its URL in `/etc/dota-arcade/platform.env` with mode 0600. Set `PLATFORM_ENV=production`, `PLATFORM_PUBLIC_ORIGIN=https://...`, `PLATFORM_LISTEN_ADDR=127.0.0.1:8080`, explicit `PLATFORM_MAX_PARTY_SIZE`, and `PLATFORM_WEB_ROOT=/opt/dota-arcade/current/web`.
2. Build `platform-server` for Linux amd64 and `web/dist` in CI or a build machine. Package them under a versioned directory `/opt/dota-arcade/releases/<build>/`; link `current` to that directory. The source checkout is never the runtime directory.
3. Install `systemd/platform-server.service` and adapt `caddy/Caddyfile.example` to the real public domain. Caddy owns TLS; Platform listens only on loopback. The platform unit runs `migrate` before `serve`, so a failed migration prevents startup.
4. Configure a private libpq service file for the same database and set `PGSERVICE` (and `PGSERVICEFILE` if needed), so the backup command does not put a credential-bearing URL in process arguments. Back up before every update with `scripts/backup-postgres.sh` and verify the dump. See [operations](../docs/operations.md) for recovery and rollback.
5. Verify `systemctl status`, the local `/healthz`, public HTTPS `/healthz`, player page and admin login. Do not expose PostgreSQL or Platform loopback HTTP directly to the internet.

## Linux game node

Install `node-controller`, `content-tool`, official fixed d2core v0.1.1 and `BUILD.json` in a versioned native directory, with `current` pointing at the active build. Keep the Controller JSON and node Secret outside that directory under `/etc/dota-arcade-node/`; allow the shared ordinary runtime account to read the Secret. Keep d2core data, logs, templates and ContentRoot on persistent storage. Adjust `configs/examples/node-controller.linux.json.example` for actual local paths and network facts. Install the three Linux files in `systemd/` and the executable `start-d2core-manager.sh` into the active node build.

`start-d2core-manager.sh` reads the Controller JSON with `jq` and passes its `network.localPortMin`/`localPortMax` to fixed d2core `serve`. This makes both components use the same local game-port bounds. The Controller service follows the manager and retries on failure. Neither unit configures firewall, NAT, Steam or Dota.

## Windows game node

Place `node-controller.exe`, official d2core v0.1.1 `d2core.exe` and its `BUILD.json` under `C:\ProgramData\DotaArcadeNode\bin`, with JSON and Secret under `config`, plus persistent `data`, `content`, `templates` and `logs`. Copy the three scripts in `windows/` to `bin`. Edit `configs/examples/node-controller.windows.json.example` and validate all local paths and network mappings.

Run `install-node.ps1 -Root C:\ProgramData\DotaArcadeNode -ValidateOnly` first. After checking the output and runtime account, run it without `-ValidateOnly` in an elevated PowerShell session and supply that account's credential at the prompt. It registers two startup scheduled tasks under the same account. Start the d2core task first, then Controller, and verify task results, transcripts, Platform heartbeat, direct d2core list and Dota process state. The d2core wrapper reads the exact same Controller JSON port bounds on every start. The installer does not alter firewall, NAT, Dota files or router state.

On both systems, `content-tool` uses `CONTENT_ROOT` and `DOTA_ROOT` or equivalent `--content-root`/`--dota-root` flags. Set each Controller content binding's `metadataPath` to `<ContentRoot>/<WorkshopID>/metadata/current.json` and `currentLinkPath` to `<DotaRoot>/game/dota_addons/<WorkshopID>`. The Tool never runs automatically.
