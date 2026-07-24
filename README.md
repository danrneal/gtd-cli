# gtd-cli

The `gtd-cli` is a command-line interface that synchronizes external task providers (such as Google Tasks) with local Markdown files. This synchronization enables a terminal-based task workflow, allowing you to view and manipulate tasks directly in your terminal (or via an LLM) while staying synced with external graphical interfaces.

## Key features

The `gtd-cli` tool provides the following features:

*   **Markdown-driven workflow:** Parses local Markdown files into structured tasks, enabling seamless terminal and LLM integrations.
*   **External synchronization:** Synchronizes local tasks automatically with external UI platforms (currently supporting Google Tasks).
*   **Offline persistence:** Maintains a local SQLite database for offline access, fast retrieval, and data integrity.

## Architecture

The `gtd-cli` application acts as a central synchronization engine. It orchestrates state changes between various task management systems, which we call **providers**. 

The system currently supports the following providers:

*   **SQLite:** A local database that ensures data integrity.
*   **Markdown:** A local file parser that lets you manage tasks as plain text.
*   **Google Tasks:** A remote provider that links your local tasks to your Google account.

## Installation

Take the following steps to install `gtd-cli` on your system:

1.  Verify that your system runs Go version 1.21 or higher.
2.  Clone this repository to your local machine.
3.  Build the executable binary:

    ```bash
    go build -o gtd ./cmd/gtd
    ```

## Usage

To synchronize your tasks, invoke the `gtd` binary and provide the necessary flags to configure your providers. For example:

```bash
./gtd --db path/to/gtd.db --provider google_tasks --credentials creds.json
```

The `gtd` command supports the following flags:

*   `--db`: Path to the SQLite database. (Default: `~/.local/share/gtd/gtd.db`)
*   `--provider`: The name of the remote provider to sync with (e.g., `google_tasks`).
*   `--credentials`: Path to the provider credentials file. (Default: `~/.config/gtd/gtd_credentials.json`)
*   `--token`: Path to the OAuth token file. (Default: `~/.config/gtd/gtd_token.json`)


