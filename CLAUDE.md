# Agent Mailbox (ambox.dev)

E2E encrypted email relay for AI agents. Agents register, get an email address (`{id}@ambox.dev`), send/receive emails encrypted with hybrid RSA+AES, and manage their mailbox via API or CLI.

## Stack

- **Runtime:** Go 1.23+
- **Email:** Resend (outbound API + inbound webhooks)
- **DB:** Firestore Enterprise (MongoDB compatibility, use `go.mongodb.org/mongo-driver/v2`)
- **Storage:** GCS bucket for encrypted attachments
- **Crypto:** RSA-4096 + AES-256-GCM hybrid (stdlib `crypto/*`)
- **Classifier:** Ollama Cloud (OpenAI-compatible, `https://ollama.com/v1`)
- **Hosting:** Cloud Run (GCP project: `iconic-elevator-394020`)
- **CLI:** Node.js skill (noti/grog pattern)

## Architecture

```
Agent (CLI/SDK)  ←→  ambox API (Cloud Run)  ←→  Resend (send + inbound)
                           ↕                           
                     Firestore/MongoDB          GCS Bucket
                     (encrypted store)       (encrypted attachments)
```

## Commands

```bash
make build          # Build binary
make test           # Run unit tests
make test-int       # Run integration tests
make docker         # Build Docker image
make deploy         # Deploy to Cloud Run
```

## API

All authenticated endpoints require `Authorization: Bearer {api-key}`.

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | /v1/register | No | Create agent, return API key + RSA private key |
| POST | /v1/send | API key | Send email via Resend |
| POST | /v1/inbound | Svix sig | Resend inbound webhook |
| GET | /v1/inbox | API key | Poll encrypted inbox |
| DELETE | /v1/emails/:id | API key | Delete email |
| PUT | /v1/webhook | API key | Configure webhook URL |
| PUT | /v1/settings | API key | Update TTL, display_name |
| GET | /v1/emails/:id/attachments/:filename | API key | Download encrypted attachment |
| PUT | /v1/emails/:id/move | API key | Move email to folder |

## Crypto

- Per email: random AES-256 key → encrypt body with AES-256-GCM → wrap AES key with RSA-OAEP
- Deterministic nonces: 0x01=subject, 0x02=body, 0x03+=attachments
- Registration: generate RSA-4096, store public key only, return private key once
- API keys: argon2id hash, prefix-indexed for fast lookup
