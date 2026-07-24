# Installation

ChenWeb runs from the shared Go workspace. To get a local copy running:

1. Make sure you have **Go 1.25+**, **Node.js/Bun**, and **PostgreSQL** installed.
2. From the `ChenWeb/` directory, install frontend dependencies:
   ```bash
   cd web && bun install
   ```
3. Start the dev server (backend + frontend together):
   ```bash
   mise dev
   ```
4. The dev server auto-applies database migrations on startup and hot-reloads the Go backend on file changes.

## Configuration

Local configuration lives in `config.local.toml` and `mise.local.toml`. Copy the checked-in defaults if these files don't already exist, and adjust database connection settings to match your local PostgreSQL instance.

See **Navigating the Dashboard** for a tour of the app once it's running.
