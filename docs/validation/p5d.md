# P5D reference deployment checkpoint (in progress)

Linux Caddy/systemd examples and Windows startup-task installer are in `deploy/`. The Linux and Windows d2core startup wrappers read their port bounds from the same Controller JSON. The Windows installer checks the fixed d2core v0.1.1 build manifest, required files, Secret and port range before task registration. Example Controller JSONs are in `configs/examples/`.

Local script parser and example JSON checks pass. GitHub Actions [run 36231578148](https://github.com/L4C99/dota2-arcade-platform/actions/runs/36231578148) passed the Linux shell and Windows PowerShell syntax jobs. Actual Ubuntu and Windows installation, service restart, resource persistence and external HTTPS entry are **NOT VERIFIED**. Production deployment is outside P5 authorization. P5D is not marked PASS.
