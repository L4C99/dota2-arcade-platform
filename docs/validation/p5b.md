# P5B Content Tool checkpoint (in progress)

## Implemented

- Offline `content-tool status/prepare/switch/rollback` keeps immutable VPK releases and metadata outside the live addon directory. Linux uses a whole-directory symlink; Windows uses a directory Junction. A transition journal and explicit recovery path protect interrupted switches. The tool does not call Platform, Controller or d2core.
- Controller reads the actual linked directory and VPK SHA256, reporting `unknown` when it cannot confirm the local content.
- Local Windows tests exercise prepare, idempotence, immutable-version rejection, switch, rollback, Junction readback, interrupted transition recovery and lock handling. GitHub Actions [run 36231578148](https://github.com/L4C99/dota2-arcade-platform/actions/runs/36231578148) passed Linux and Windows Go jobs, including the same Content Tool tests on both operating systems with disposable test files.

## Real development-node verification

- The owner-supplied VPKs were copied to isolated local and per-node staging, then each copy's SHA256 was checked. Content Tool prepared both immutable releases on Linux and Windows. The new Workshop `2307479570` was switched on an empty drained Linux node and an empty drained Windows node; actual symlink/Junction status and Controller digest readback confirmed the same version and bytes.
- The pre-P5 `3564393242` layout required one-time adoption. Each node's currently linked `p1-test-v1` VPK was first copied into isolated staging and verified as SHA256 `5003e3a21346533a332dc0a1a272fefd8cec9b343a82777477902524b0f11f8b`; Content Tool prepared it as an immutable release. Windows already linked that release, so the prepared metadata was adopted after exact Junction target check. Linux preserved its original symlink outside the addon tree, then switched to a SHA-identical prepared release. Controller bindings were moved to Content Tool's current metadata, and both Controllers confirmed `p1-test-v1` afterward. Neither original VPK was overwritten.
- While drained and empty, Linux then Windows independently switched the old game to `p5-3564393242-legacy`. Controller readback confirmed SHA256 `27b4b93824c2a5ebc96ad9986954e9e6602981800085dee3805739afb4249c0c` on the updated node while the other node still reported `p1-test-v1`. A temporary official d2core instance using each node's formal old-game template reached Ready, then was explicitly stopped and fully reclaimed. Content Tool `rollback` restored `p1-test-v1` on each node. Controller readback, completed reconciliation, empty d2core lists and Resume were verified. Both immutable releases remain available.

## NOT VERIFIED

- The full single-node publication and two-node rolling business flow still requires human player/content validation before Admin `content.validate`/`content.publish`. No global current version was changed. Active old-version Allocation preservation, waiting-request version behavior and real player JoinInfo during a mixed-version rollout remain covered by automation but have not yet been exercised in this real rollout. P5B is not marked PASS.
