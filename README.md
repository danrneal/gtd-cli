# gtd.nvim

A robust "Getting Things Done" (GTD) plugin for Neovim, built with a Lua frontend and a high-performance Go backend using SQLite for persistence.

## Architecture

This project consists of three main components:

1.  **Neovim Plugin (Lua):** Handles the user interface, commands, and communication with the backend.
2.  **Sidecar/Daemon (Go):** A standalone Go application that manages the business logic and database interactions. It can run as a sidecar process spawned by Neovim or as a system-wide daemon.
3.  **Storage (SQLite):** A lightweight, file-based database ensures data integrity and portability.

## Goals

-   **Reliability:** High test coverage (aiming for 100%) for both Lua and Go components.
-   **Performance:** leveraging Go for heavy lifting and SQLite for efficient data retrieval.
-   **Usability:** Seamless integration with the Neovim ecosystem.

## Installation

*(Coming soon)*

## Development

We follow strict development practices:

-   **Testing:** All features and refactors must be accompanied by comprehensive unit tests.
-   **Documentation:** Documentation is updated alongside code changes.
-   **Git Hygiene:** Regular commits with clear messages, ensuring a stable main branch.

### Prerequisites

-   Neovim (>= 0.9.0)
-   Go (>= 1.21)
-   SQLite3
