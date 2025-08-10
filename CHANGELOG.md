# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

### Added
- **Project Structure**: Initialized a Go project with a modular structure (`cmd`, `internal`).
- **Configuration Management**: Implemented configuration loading from environment variables using `godotenv` and `caarlos0/env`.
- **Telegram Bot Integration**: Added a basic Telegram bot using `go-telegram-bot-api`.
- **LLM Client Abstraction**: Created a common `llm.Client` interface to support multiple LLM providers.
- **OpenAI Client**: Implemented a client for OpenAI-compatible APIs.
- **YandexGPT Client**: Implemented a client for YandexGPT using `Morwran/yagpt`.
- **LLM Provider Selection**: Added the ability to choose the LLM provider (`openai` or `yandex`) via the `LLM_PROVIDER` environment variable.
- **User Authorization**: Created a service to restrict bot access to a list of allowed user IDs.
- **Flexible API Endpoint**: Added the ability to specify a custom `BaseURL` for the LLM API via the `OPENAI_BASE_URL` environment variable.
- **Flexible Model Selection**: Added the ability to specify the LLM model name via the `OPENAI_MODEL` environment variable, with `gpt-3.5-turbo` as the default.
- **Enhanced Unauthorized User Handling**: The bot now replies to unauthorized users with a "request sent for review" message and logs their user ID and username.
- **.env Loading**: Improved `.env` loading to search multiple common locations (`.env`, `../.env`, `cmd/bot/.env`).
- **System Prompt**: Added `SYSTEM_PROMPT_PATH` and support for a system prompt file; passed to both OpenAI and YaGPT clients.
- **Logging**: Added logging of incoming user messages and LLM responses (model name and token usage).
- **Response Meta Line**: Bot prepends each answer with `[model=..., tokens: prompt=..., completion=..., total=...]`.
- **Per-user Conversation History**: Implemented a thread-safe history manager; the context is isolated per user and included in LLM requests.
- **Reset Context Button**: Added an inline button "Сбросить контекст" in Telegram; clears only the requesting user's history.
- **LLM Context Refactor**: Refactored `llm.Client` interface to `Generate(ctx, []llm.Message)` and updated OpenAI/YaGPT clients to accept full message history.
- **History Summary**: Added an inline button "История" to request a summary of the user's conversation with the assistant; the summary is logged, sent to the user (with meta line), and appended back to the user's history.
- **Storage Abstraction**: Introduced `storage.Recorder` and `storage.Event` for pluggable persistence.
- **File Logger (JSONL)**: Implemented file-based recorder writing one JSON per line to `LOG_FILE_PATH` (default `logs/log.jsonl`).
- **History Restore**: On startup, the bot preloads events from the recorder and reconstructs per-user history.
- **Config**: Added `LOG_FILE_PATH` env var to configure the path for JSONL log file.
- **Admin Approval Flow**: Added `ADMIN_USER` env var. Unauthorized user requests are sent to the admin with inline buttons "разрешить"/"запретить".
- **Allowlist Storage**: Introduced `auth.Repository` abstraction and file-based JSON allowlist (`ALLOWLIST_FILE_PATH`, default `data/allowlist.json`) storing `{id, username, first_name, last_name}`; approvals/denials update file and in-memory state.
- **/start Improvements**: Added welcome message with hints about inline buttons; auto-sends access request to admin and informs the user.
- **Pending Storage**: Added file-based pending repository (`PENDING_FILE_PATH`, default `data/pending.json`) to persist pending access requests across restarts.
- **Admin Pending Commands**: Added `/pending` to list pending users and `/approve <user_id>`, `/deny <user_id>` to allow/deny; updates pending file and allowlist on the fly.
- **Pending UX**: If a user has already requested access, bot no longer spams admin and informs the user to wait for approval.
- **Markdown Formatting**: Added `MESSAGE_PARSE_MODE` env var. All outgoing messages support Markdown/MarkdownV2/HTML parse modes.
- **CI**: Added GitHub Actions workflow to build and run tests on pushes/PRs to `main` and `develop`.
- **Unit Tests**: Added tests for history, storage, auth, pending, and basic telegram logic with mocks.
- **OpenRouter Support**: Added optional OpenRouter headers (`OPENROUTER_REFERRER`, `OPENROUTER_TITLE`) and README instructions; set `OPENAI_BASE_URL=https://openrouter.ai/api/v1` and supply OpenRouter model names.
