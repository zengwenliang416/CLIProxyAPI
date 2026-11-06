# Vendored Management Center

This directory vendors the open-source CLIProxyAPI management center so the
panel version is controlled by this repository.

- Upstream repository: https://github.com/router-for-me/Cli-Proxy-API-Management-Center
- Imported commit: `953fb2444350c6c36052dda46cac0abce6b23ea6`
- Imported on: 2026-11-06
- Management center release: `v1.27.3`
- Backend baseline: CLIProxyAPI `v8.0.57`, `c2f402b45643f0740d70b9010fd7d8d0aab7ef65`
- License: MIT, see `LICENSE`

The Docker build runs `bun install --frozen-lockfile` and `bun run build`, then
copies the generated single-file `dist/index.html` into the CLIProxyAPI image
as `/CLIProxyAPI/static/management.html`.

The backend does not download or replace this file at runtime. Update this
directory deliberately, review the frontend changes, run its verification
commands, and deploy the resulting CLIProxyAPI commit.
