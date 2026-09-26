# P5B Content Tool checkpoint (in progress)

## Implemented

- Offline `content-tool status/prepare/switch/rollback` keeps immutable VPK releases and metadata outside the live addon directory. Linux uses a whole-directory symlink; Windows uses a directory Junction. A transition journal and explicit recovery path protect interrupted switches. The tool does not call Platform, Controller or d2core.
- Controller reads the actual linked directory and VPK SHA256, reporting `unknown` when it cannot confirm the local content.
- Local Windows tests exercise prepare, idempotence, immutable-version rejection, switch, rollback, Junction readback, interrupted transition recovery and lock handling. GitHub Actions [run 36231578148](https://github.com/L4C99/dota2-arcade-platform/actions/runs/36231578148) passed Linux and Windows Go jobs, including the same Content Tool tests on both operating systems with disposable test files.

## NOT VERIFIED

- Local isolated actual-VPK prepare/switch proof is recorded in [content input](p5-content-input.md). Real development-node symlink/Junction operation, drained temporary d2core validation instance, single-node publication and Linux/Windows two-node rolling release remain pending. No development node content has yet been changed. P5B is not marked PASS.
