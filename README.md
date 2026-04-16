# Agent Mailbox (ambox.dev)

E2E encrypted email relay for AI agents. Register an agent, get an email address, send and receive emails — all encrypted at rest with hybrid RSA+AES. The server never stores plaintext.

## How it works

```
Agent (CLI)  <-->  ambox API (Cloud Run)  <-->  Resend (send + receive)
                         |
                   Firestore/MongoDB
                   (encrypted storage)
```

1. **Register** — agent gets `{id}@ambox.dev`, an API key, and an RSA-4096 private key (delivered once, never stored server-side)
2. **Send** — agent sends email via API, server relays through Resend, saves encrypted copy
3. **Receive** — external email arrives at Resend, webhook hits ambox, server fetches body, encrypts with agent's public key, stores ciphertext
4. **Read** — agent polls inbox, downloads encrypted emails, decrypts locally with private key
5. **Classify** — incoming emails are classified (inbox/important/transactional/notification/spam) via LLM before encryption

## Encryption

Every email is encrypted with **AES-256-GCM** using a per-message random key. The AES key is wrapped with the agent's **RSA-4096 public key** (OAEP/SHA-256). The server stores only ciphertext + wrapped key. Decryption happens exclusively on the client.

Deterministic nonces: `0x01` = subject, `0x02` = body, `0x03+` = attachments.

## API

Base URL: `https://ambox.dev/v1`

All authenticated endpoints require `Authorization: Bearer {api-key}`.

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `POST` | `/register` | Rate-limited by IP | Create agent, returns API key + RSA private key (once) |
| `POST` | `/send` | API key | Send email via Resend |
| `GET` | `/inbox` | API key | Poll encrypted emails |
| `DELETE` | `/emails/{id}` | API key | Delete email |
| `PUT` | `/emails/{id}/move` | API key | Move email to folder |
| `PUT` | `/webhook` | API key | Configure push notification URL |
| `PUT` | `/settings` | API key | Update TTL, display name |
| `GET` | `/health` | None | Health check |

### POST /register

```json
{
  "agent_id": "my-agent",
  "display_name": "My Agent",
  "webhook_url": "https://my-server.com/hook",
  "ttl_seconds": 604800
}
```

Response (returned exactly once):

```json
{
  "agent_id": "my-agent",
  "email": "my-agent@ambox.dev",
  "api_key": "amk_live_...",
  "private_key_pem": "-----BEGIN PRIVATE KEY-----\n..."
}
```

### POST /send

```json
{
  "to": ["alice@example.com"],
  "subject": "Hello",
  "body_html": "<p>Hello from my agent</p>",
  "body_text": "Hello from my agent"
}
```

### GET /inbox

Query params: `folder` (inbox|sent|important|transactional|notification|spam), `limit`, `since` (RFC3339), `cursor`.

Returns encrypted emails. Decrypt client-side:
1. RSA-OAEP decrypt `wrapped_key` with private key → AES key
2. AES-256-GCM decrypt `subject_encrypted` with nonce `0x01`
3. AES-256-GCM decrypt `body_encrypted` with nonce `0x02`

## CLI

Install and use via the Claude Code skill, or standalone:

```bash
# Register
node index.js register --agent-id my-agent --endpoint https://ambox.dev

# List agents
node index.js agents

# Check inbox (decrypts locally, saves to ~/.ambox/agents/{id}/inbox/)
node index.js inbox

# Read email
node index.js read msg_abc123

# Send email
node index.js send alice@example.com "Hello" --body "Message body"

# Use specific agent
node index.js --agent other-agent inbox
```

### Local storage

Emails are decrypted and stored locally per agent, organized by folder and timestamp:

```
~/.ambox/
├── agents/
│   ├── my-agent/
│   │   ├── config.json
│   │   ├── private.pem
│   │   ├── inbox/
│   │   │   └── 20260416-152533_msg_abc123/
│   │   │       └── email.json
│   │   ├── sent/
│   │   └── important/
│   └── other-agent/
│       └── ...
└── default
```

## Stack

- **Runtime:** Go 1.23+
- **Email:** Resend (outbound API + inbound webhooks)
- **Database:** Firestore Enterprise (MongoDB compatibility)
- **Crypto:** Go stdlib `crypto/*` (RSA-4096, AES-256-GCM, argon2id)
- **Classifier:** Ollama Cloud (OpenAI-compatible API)
- **Hosting:** Google Cloud Run
- **CLI:** Node.js (ES modules, zero framework)

## Development

```bash
make build          # Build binary
make run            # Run locally
make test           # Unit tests
make test-race      # Tests with race detector
make docker         # Build Docker image
make deploy         # Deploy to Cloud Run
```

Environment variables:

| Variable | Description |
|----------|-------------|
| `MONGODB_URI` | MongoDB connection string |
| `MONGODB_DATABASE` | Database name |
| `RESEND_API_KEY` | Resend API key (full access) |
| `RESEND_WEBHOOK_SECRET` | Resend webhook signing secret |
| `EMAIL_DOMAIN` | Email domain (default: ambox.dev) |
| `OLLAMA_API_KEY` | Ollama Cloud API key |
| `OLLAMA_BASE_URL` | Ollama endpoint (default: https://ollama.com/v1) |
| `OLLAMA_MODEL` | Classification model (default: qwen2.5:7b) |
| `GCS_BUCKET` | GCS bucket for attachments (optional) |
| `PORT` | Server port (default: 8080) |

## License

MIT
