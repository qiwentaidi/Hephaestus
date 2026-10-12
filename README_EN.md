<div align="center">
<a href="https://github.com/qiwentaidi/Hephaestus"><img src="assets/appicon.png" width="120"/></a>
</div>
<h1 align="center">Hephaestus</h1>

<div align="center">
  <a href="README.md">中文</a> •
  <a href="README_EN.md">English</a>
</div>
<br />

Hephaestus is a security testing platform that combines asset collection, attack surface assessment, vulnerability validation, and AI penetration testing conversations. It provides a Web service, a desktop client, and a standalone scanning Agent, with support for local scanning, combined tasks, and execution across distributed nodes.

- Web service downloads: [Releases](https://github.com/qiwentaidi/Hephaestus/releases)
- Desktop client: [Hephaestus-Desktop](https://github.com/qiwentaidi/Hephaestus-Desktop)
- Scanning nodes and command-line tools: [Hephaestus-Agent](https://github.com/qiwentaidi/Hephaestus-Agent)
- License registration: [Official website](http://47.97.59.120)

## Highlights

- **Combined scans**: Combine website, port, directory, subdomain, and JS/API scanning as needed, and review progress and results within a single task.
- **AI penetration testing conversations**: Use tools to investigate authorized targets and record findings, reproduction steps, and evidence for review.
- **Distributed execution**: Run tasks locally or on remote Agents, with centralized management of nodes, tasks, and scan results.

## Main Features

| Module | Features |
| --- | --- |
| Attack Surface Assessment | Website fingerprinting, vulnerability scanning, port and service identification, weak password testing, directory, subdomain, and JS/API scanning |
| Task Management | Concurrency control, pause and resume, cancellation, history, scheduling, parameter templates, and result search |
| AI Workbench | Projects, sessions, tool calls, image input, testing records, and evidence review |
| AI Batch Tasks | Batch task creation, queue management, and execution status |
| Vulnerability Management | Consolidate scan and AI findings, review reproduction steps and evidence, and track manual remediation |
| Asset Management | Centralized asset results, topology, and map views |
| Search Engines | FOFA, Hunter, and Quake queries and aggregated searches, with history, quota information, and downloads |
| Company Information | Collect company information and related assets, with individual or merged Excel exports |
| Request Repeater | Edit HTTP requests, inspect responses, save tabs, and refine and debug Nuclei templates |
| PoC Management | Nuclei template search, editing, additional directories, debugging, and out-of-band (OOB) connectivity testing |
| Analysis Tools | Encoding and decoding, JWT, API documentation testing, text comparison, and antivirus identification; mini-program decompilation on desktop |
| Data and Credentials | Database connections, data inspection, and key management |

## Choose a Deployment Mode

| Mode | Database and Runtime | Use Cases |
| --- | --- | --- |
| Web Service | PostgreSQL by default, with an explicit SQLite configuration option; frontend static assets are embedded in the backend; default port: `18181` | Browser access, team use, or server deployment |
| Desktop Client | SQLite by default, with PostgreSQL support; includes a local service and a separate settings window | Single-machine use and integration with local tools |
| Scanning Agent | Persistent nodes use a shared PostgreSQL database to claim tasks, send heartbeats, and return results | Scanning across multiple nodes or executing tasks on remote networks |

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

### 3. Open the Web Interface and Complete Activation and Setup

Save the configuration, restart the application, and open [http://localhost:18181](http://localhost:18181). For server deployments, replace `localhost` with the server address. If you changed the port, use the value configured in `web.port`.

1. Follow the page instructions to activate with a license key online or import an offline license. Use the official website linked above to register for a license.
2. After activation, a new instance prompts you to create an administrator account. If the instance has already been initialized, sign in.

### 4. Prepare Your First Scan or AI Session

After signing in, configure the features you plan to use:

- **Scanning resources**: Open **System Settings → System Update** and use **Config Update** to download or update resources such as PoCs, directory wordlists, and weak password dictionaries.
- **Search engines**: Before using services such as FOFA, Hunter, or Quake, enter their keys under **System Settings → API Keys**.
- **AI Workbench**: Configure the provider, API URL, key, and model under **System Settings → AI Config**, then test the connection.
- **Remote nodes**: Before using Agents, prepare a PostgreSQL database shared by the main service and nodes, then follow the deployment instructions in the [Agent repository](https://github.com/qiwentaidi/Hephaestus-Agent).

Once the required configuration is ready, create a scan task or start an AI Workbench session with your authorized target and testing scope.

## Quick Start: Desktop Client

Download the installer for your operating system and CPU architecture from the [desktop repository](https://github.com/qiwentaidi/Hephaestus-Desktop).

Install and launch the client, then follow the page instructions to activate your license and create an administrator account or sign in. Prepare scanning resources, API keys, and AI configuration using the same steps as the Web service.

- Default configuration file: `~/.config/hephaestus/config-desktop.yaml`.
- Default database: `~/.config/hephaestus/data/hephaestus.db`. You can also switch to PostgreSQL.
- The embedded service listens on `127.0.0.1:38456` by default. If the port is occupied, it uses a temporary port. Set `HEPHAESTUS_DESKTOP_BACKEND_PORT` to specify the preferred port.
- The gear button at the top opens a separate settings window that shares settings components with the in-app settings page.
- Desktop features include a local launcher and mini-program decompilation.

The Web service and desktop client use separate configuration files but share the same default configuration directory. To keep their data separate, set `HEPHAESTUS_CONFIG_DIR` to a different directory for each.

## Frequently Asked Questions

### Browser Screenshots Are Unavailable

Screenshots require a working Chromium browser and its runtime dependencies. On Linux servers and minimal containers, check that the browser, fonts, and graphics dependencies are installed.

### AI Local Commands Cannot Run

Local commands and interactive terminals in AI sessions run with filesystem write isolation. Output files are written to the session working directory. macOS uses `sandbox-exec`, while Linux uses Bubblewrap (`bwrap`). Execution is refused if the isolation tool is missing or initialization fails.

### TCP Discovery and SYN Permissions

TCP discovery uses Connect mode by default. When choosing SYN mode, check system permissions, packet capture dependencies, and SYN capability status. Enable the required permissions through the platform's authorization interface when needed.
