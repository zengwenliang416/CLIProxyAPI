# Vendored Management Center

This directory vendors the open-source CLIProxyAPI management center so the
panel version is controlled by this repository.

- Upstream repository: https://github.com/router-for-me/Cli-Proxy-API-Management-Center
- Imported commit: `64b4f5aed19b8f791eb21eb9e785ea379ea05b44`
- Imported on: 2026-10-30
- Management center release: `v1.26.7`, plus 3 commits on main after that tag
- Backend baseline: CLIProxyAPI `v8.0.47`, `90c447d3a8727d115aa7552ec93ce023736ed492`
- License: MIT, see `LICENSE`

The Docker build runs `bun install --frozen-lockfile` and `bun run build`, then
copies the generated single-file `dist/index.html` into the CLIProxyAPI image
as `/CLIProxyAPI/static/management.html`.

The backend does not download or replace this file at runtime. Update this
directory deliberately, review the frontend changes, run its verification
commands, and deploy the resulting CLIProxyAPI commit.
