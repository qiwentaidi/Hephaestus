<div align="center">
<a href="https://github.com/qiwentaidi/Hephaestus"><img src="assets/appicon.png" width="120"/></a>
</div>
<h1 align="center">Hephaestus</h1>

<div align="center">
  <a href="README.md">中文</a> •
  <a href="README_EN.md">English</a>
</div>

Hephaestus is a security testing platform that combines asset collection, attack surface assessment, vulnerability validation, and AI penetration testing conversations. It provides a Web service, a desktop client, and a standalone scanning Agent, with support for local scanning, combined tasks, and execution across distributed nodes.

- License registration: [Official website](http://47.97.59.120)
- Desktop client: [Hephaestus-Desktop](https://github.com/qiwentaidi/Hephaestus-Desktop)
- Scanning nodes and command-line tools: [Hephaestus-Agent](https://github.com/qiwentaidi/Hephaestus-Agent)

## Main Features

| Module | Features |
| --- | --- |
| Attack Surface Assessment | Website fingerprinting and vulnerability scanning, port and service identification, weak password testing, directory scanning, subdomain collection, and JS/API asset discovery and vulnerability detection; combine scanning modules into tasks as needed |
| Task Management | Local or Agent execution, concurrency control, pause and resume, cancellation, scan history, scheduling, parameter templates, and result search |
| AI Workbench | Project and session management, standalone penetration testing conversations, tool calls, image input, fact and vulnerability records, exploration paths, evidence retrieval, and side conversations |
| AI Batch Tasks | A dedicated interface for creating batch tasks and managing queues and execution |
| Vulnerability Management | Review scan and AI findings, reproduction steps, and evidence, and track manual remediation status |
| Asset Management | View assets centrally through the asset center, asset topology, and map views |
| Search Engines | Individual and aggregated searches with FOFA, Hunter, and Quake, including query history, quota information, and a download center |
| Company Information | Collect company information and related assets, review task results, and export individual or merged Excel files |
| Request Repeater | Edit and send HTTP requests, inspect responses, persist tabs, and refine and debug Nuclei templates |
| PoC Management | Search and edit Nuclei templates, manage additional template directories, debug templates, and test out-of-band (OOB) connectivity |
| Analysis Tools | Encoding and decoding, JWT tools, API documentation testing, text comparison, and antivirus identification from process lists; the desktop client also provides mini-program decompilation |
| Data and Credentials | Database connections and data inspection, and key management |

## Choose a Deployment Mode

| Mode | Database and Runtime | Use Cases |
| --- | --- | --- |
| Web Service | PostgreSQL by default, with an explicit SQLite configuration option; frontend static assets are embedded in the backend; default port: `18181` | Browser access, team use, or server deployment |
| Desktop Client | SQLite by default, with PostgreSQL support; includes a local service and a separate settings window | Single-machine use and integration with local tools |
| Distributed Scanning Agent | Persistent nodes use a shared PostgreSQL database to claim tasks, send heartbeats, and return results | Scanning across multiple nodes or executing tasks on remote networks |

## Quick Start: Web Service

### 1. Download the Correct Binary

Download the version for your operating system and CPU architecture from [Releases](https://github.com/qiwentaidi/Hephaestus/releases), extract it, and launch it from a terminal. For example, on Linux AMD64:

```bash
chmod +x ./hephaestus-linux-amd64
./hephaestus-linux-amd64
```

On its first run, the Web service generates `~/.config/hephaestus/config.yaml`. If the default PostgreSQL database is not ready, the application reports a database connection failure. Complete the configuration below, then restart the application.

### 2. Configure the Database

#### Option A: PostgreSQL (Web Default; Required for Distributed Deployment)

You can use an existing PostgreSQL instance or start a local test instance with Docker:

```bash
docker run -d \
  --name hephaestus-postgres \
  -e POSTGRES_USER=postgres \
  -e POSTGRES_PASSWORD='replace-with-your-password' \
  -e POSTGRES_DB=hephaestus \
  -p 127.0.0.1:5432:5432 \
  -v hephaestus-pgdata:/var/lib/postgresql/data \
  postgres:15
```

Enter the matching connection details in `config.yaml`:

```yaml
web:
  port: 18181
database:
  type: postgres
  host: 127.0.0.1
  port: "5432"
  user: postgres
  password: "replace-with-your-password"
  dbname: hephaestus
  sslmode: disable
```

#### Option B: SQLite (Standalone Builds with SQLite Support)

No PostgreSQL instance is needed. Configure `config.yaml` as follows:

```yaml
web:
  port: 18181
database:
  type: sqlite
```

If `database.path` is omitted, the database is stored at `~/.config/hephaestus/data/hephaestus.db` by default. Set `path` to specify an absolute location. SQLite standalone mode does not support the current distributed Agent deployment, which requires a shared database.

## Desktop Client

Download the installer for your operating system and CPU architecture from the [desktop repository](https://github.com/qiwentaidi/Hephaestus-Desktop).

- Default configuration file: `~/.config/hephaestus/config-desktop.yaml`.
- Default database: `~/.config/hephaestus/data/hephaestus.db`. You can also switch to PostgreSQL.
- The embedded service listens on `127.0.0.1:38456` by default. If the port is occupied, it uses a temporary port. Set `HEPHAESTUS_DESKTOP_BACKEND_PORT` to specify the preferred port.
- The gear button at the top opens a separate settings window that shares settings components with the in-app settings page.
- Desktop features include a local launcher and mini-program decompilation.

The Web service and desktop client use separate configuration files but share the same default configuration directory. To keep their data separate, set `HEPHAESTUS_CONFIG_DIR` to a different directory for each.

## Frequently Asked Questions

### 1️⃣ Browser Screenshots Are Unavailable

Screenshots require a working Chromium browser and its runtime dependencies. On Linux servers and minimal containers, check that the browser, fonts, and graphics dependencies are installed.

### 2️⃣ AI Local Commands Cannot Run

Local commands and interactive terminals in AI sessions run with filesystem write isolation. Output files are written to the session working directory. macOS uses `sandbox-exec`, while Linux uses Bubblewrap (`bwrap`). Execution is refused if the isolation tool is missing or initialization fails.

### 3️⃣ TCP Discovery and SYN Permissions

TCP discovery uses Connect mode by default. When choosing SYN mode, check system permissions, packet capture dependencies, and SYN capability status. Enable the required permissions through the platform's authorization interface when needed.
