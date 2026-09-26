# Owner-confirmed P5 development content input

The project owner supplied real content for both development nodes on 2026-09-26. The temporary catalog names may be changed before external testing. The original assets remain untouched; local content validation uses copies under Git-ignored `.local/`, and node release preparation must use separate staged copies.

| Workshop | Proposed immutable ContentVersion ID | VPK bytes | SHA256 | Role |
| --- | --- | ---: | --- | --- |
| `2307479570` | `p5-2307479570-v1` | 344,793,314 | `1e086308024da977f0ce47d61aaa3cb411ca29a51f6ece7a38f607a7f74187f2` | Second test ArcadeGame |
| `3564393242` | `p5-3564393242-legacy` | 481,987,687 | `27b4b93824c2a5ebc96ad9986954e9e6602981800085dee3805739afb4249c0c` | Distinct older content for an explicit roll and rollback |

The currently deployed `3564393242` version is `p1-test-v1`, SHA256 `5003e3a21346533a332dc0a1a272fefd8cec9b343a82777477902524b0f11f8b`. The owner-supplied legacy VPK differs from it; its digest also matches the pre-P3D Windows addon documented in the prior development inventory. The proposed legacy ID describes the bytes without implying they are newer.

Temporary new game display name: `P5 测试游廊`. Its three provided schema-1 templates use `map custom`, `map dota`, and `map hard`, respectively, with `customgamemode="2307479570"`. Each preserves the Steam login success Ready condition and a 300-second startup ceiling. TemplateRevision IDs are `p5-2307479570-custom-300s`, `p5-2307479570-dota-300s`, and `p5-2307479570-hard-300s`, with one GamePreset per template and `max_players=10`. Node-specific executable, working directory and cfg paths were adapted only in isolated copies, then all three copies passed fixed d2core v0.1.1 static checks on both development operating systems.

Local Windows Content Tool proof used only the isolated VPK copies: `prepare` and `switch` of `2307479570` reported the proposed version, expected digest and confirmed current link; `prepare` of `3564393242` reported the legacy digest. This is a local file-operation result, not a development-node or player acceptance result.

Subsequent real development-node preparation, Controller hash readback, fixed-d2core Ready/full-reclaim checks, human entry of all three new-game modes, publication of `p5-2307479570-v1`, and old-game switch/rollback are recorded separately in [P5A](p5a.md) and [P5B](p5b.md). No original owner-supplied VPK or template was passed directly to Content Tool or installed in place. The old game's legacy version remains outside human gameplay acceptance.
