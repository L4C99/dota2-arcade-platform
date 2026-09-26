# P5B Content Tool checkpoint (in progress)

## Implemented

- Offline `content-tool status/prepare/switch/rollback` keeps immutable VPK releases and metadata outside the live addon directory. Linux uses a whole-directory symlink; Windows uses a directory Junction. A transition journal and explicit recovery path protect interrupted switches. The tool does not call Platform, Controller or d2core.
- Controller reads the actual linked directory and VPK SHA256, reporting `unknown` when it cannot confirm the local content.
- Local Windows tests exercise prepare, idempotence, immutable-version rejection, switch, rollback, Junction readback, interrupted transition recovery and lock handling.

## NOT VERIFIED

- Real Linux symlink operation, actual VPK preparation/switch, drained temporary d2core validation instance, single-node publication and Linux/Windows two-node rolling release. These require the owner's confirmed source VPK and target version; no development node content has been changed. P5B is not marked PASS.
