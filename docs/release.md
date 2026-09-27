# RC1 build and package procedure

Use a clean checkout of the selected full Platform SHA. Run with Go >=1.27.1,
Node 22/npm and Python >=3.10:

```text
python tools/release.py --version v1.0.0-rc1 --output dist/rc1
python tools/release_smoke.py dist/rc1
```

The output directory must not already exist. The script refuses dirty source,
builds five binaries with automatic Go VCS metadata, runs clean npm ci, lint,
typecheck, tests and build, and checks source identity again at completion.
It does not tag, publish, deploy, download d2core or touch Dota. `dist/` is ignored.
Run smoke on both native operating systems; cross-compilation alone is not a
runtime check. All three commands expose JSON `version` with exact gitCommit,
gitDirty, UTC buildTime, OS/arch and Go version. Unknown development identity is
not falsely represented as clean. Controller `check --config ABS` validates
configuration and Secret format offline; it does not contact Platform/d2core,
validate actual game content or prove node readiness.

`MANIFEST.json` records filenames, byte sizes, SHA256 and source SHA; `SHA256SUMS`
covers every artifact and the manifest. Package `PLATFORM-BUILD.json` also records
Node API 1, latest migration 19 and fixed d2core dependency. `web/BUILD.json`
identifies the matching production bundle. No player version UI is added.
`MIGRATIONS-SHA256SUMS` records embedded SQL bytes; 0001–0019 remain immutable.

| Package | Runtime payload |
| --- | --- |
| control-plane-linux-amd64.tar.gz | platform-server, web/, embedded migrations, docs/, deploy/, configs/examples/, licenses and manifests |
| game-node-linux-amd64.tar.gz | node-controller, content-tool, start-d2core-manager.sh, reference systemd/config/docs and notices |
| game-node-windows-amd64.zip | bin/node-controller.exe, bin/content-tool.exe, bin/Windows scripts, reference config/docs and notices |
| web-any.tar.gz | production web/, identity and notices |

The five standalone executable files are also checksummed. Distribute them with
the accompanying package licenses/notices, not as notice-free standalone files.
Linux tar entries preserve executable mode 0755. Public reference docs/configs
are allowlisted from tracked files; private runtime config, .local, node_modules,
game assets, dumps and credentials are not packaged. Keep all `licenses/`, root
LICENSE, THIRD_PARTY_NOTICES.md and DEPENDENCIES.json with deployed packages.
The latter inventories compiled Go modules and the locked npm graph; build/test
dependencies are not included as executable runtime material.

The workflow uploads packages as temporary CI artifacts, never a GitHub Release.
The final SHA's CI manifest is the authoritative artifact identity for that run.
Local rebuilds have their own buildTime and checksums; do not mix manifests.
Record the final SHA and run URL in the handoff. A commit cannot contain its own
SHA or the SHA-dependent binary checksums: the versioned validation record points
to this external, checksummed manifest instead of claiming a self-referential hash.

## Fixed d2core retrieval

Obtain the appropriate ZIP and checksum from the official
[v0.1.1 Release](https://github.com/L4C99/dota2-arcade-dedicated-core/releases/tag/v0.1.1).
Expected ZIP SHA256 at RC1 licensing verification:

```text
58bc1e1425466dd207e90c6ab93cb9e3ee1debfc0de7992d7fca6261df1f082c  d2core-v0.1.1-linux-amd64.zip
a930ee5ae4a5f7aa21f51d7bc6ad3b0e876f4c967f950455bd46cf86d3d7ae82  d2core-v0.1.1-windows-amd64.zip
```

Cross-check the official SHA256SUMS, BUILD.json and `d2core version --json`:
version 0.1.1, gitCommit 988720ad85af1f0d97bfe98ec4da4fcbb070beea,
matching buildTime, gitDirty=false. Keep d2core's BUILD.json distinct from
Platform's PLATFORM-BUILD.json. Do not substitute main or a renamed RC package.
The licensing evidence at db246b2bcce888b87d7854bb12012ea4e90e82cb explicitly
covers this historical source/client; it changes no runtime/protocol baseline.

## Known limitations and next gates

- Unknown create without trustworthy IDs has no safe automatic no-effect proof
  in fixed d2core. V1 retains fail-closed quarantine, Owner escape, occupied
  capacity/history and separately authorized Agent-operated node retirement or
  rebuild. Capacity may be lost permanently. This is not automatic recovery.
- Windows public Steam/steamchina entry: NOT VERIFIED.
- Fresh Ubuntu reference installation: NOT VERIFIED; later environment gate.
- Windows scheduled-task unattended installation/reboot: NOT VERIFIED.
- Production TLS/Secrets, database, recovery and rollout: NOT VERIFIED.
- Two or more real humans, same Party and real instance, formal Player Flow,
  actual play and stop/next game/full reclaim: DEFERRED TO A.7; blocks final
  v1.0.0 Release. Two sessions or bots do not satisfy it.

After RC1, separate Owner authorization is required for production catalog/names,
App570 check, private branding, fresh deployment, Windows unattended/public-entry
checks as applicable, Owner RC2 smoke and the A.7 real multiplayer gate.
