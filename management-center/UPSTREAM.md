# Vendored Management Center

This directory vendors the open-source CLIProxyAPI management center so the
panel version is controlled by this repository.

- Upstream repository: https://github.com/router-for-me/Cli-Proxy-API-Management-Center
- Imported commit: `b87b9487f63e08ad97b1fb4e7c17b4adb811b922`
- Imported on: 2026-09-30
- Management center release: `v1.25.0`
- Backend baseline: CLIProxyAPI `v8.0.4`, `d33f63f8e3d98428440ebca5a5b6a981a61ff71e`
- License: MIT, see `LICENSE`

The Docker build runs `bun install --frozen-lockfile` and `bun run build`, then
copies the generated single-file `dist/index.html` into the CLIProxyAPI image
as `/CLIProxyAPI/static/management.html`.

The backend does not download or replace this file at runtime. Update this
directory deliberately, review the frontend changes, run its verification
commands, and deploy the resulting CLIProxyAPI commit.
