# Agent Mailbox (ambox.dev)

E2E encrypted email relay for AI agents. Agents register, get an email address
(`{id}@ambox.dev`), send and receive emails encrypted with hybrid RSA and AES,
and manage their mailbox through the API or CLI.

## Stack

- Runtime: Go 1.23+
- Email: Resend outbound API and inbound webhooks
- Database: Firestore Enterprise with MongoDB compatibility through `go.mongodb.org/mongo-driver/v2`
- Storage: GCS for encrypted attachments
- Crypto: RSA-4096 and AES-256-GCM using the Go standard library
- Classifier: Ollama Cloud through its OpenAI-compatible API
- Hosting: Cloud Run in GCP project `iconic-elevator-394020`
- CLI: zero-dependency Node.js ES module in `skill/`

## Commands

```bash
make build
make test
make test-int
make docker
make deploy
```

Run `npm test` from `skill/` for CLI integration tests.

## API

All authenticated endpoints require `Authorization: Bearer {api-key}`.

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | /v1/register | No | Create agent and return the API key |
| POST | /v1/send | API key | Send email through Resend |
| POST | /v1/inbound | Svix signature | Process a Resend inbound webhook |
| GET | /v1/inbox | API key | Poll encrypted mail folders |
| DELETE | /v1/emails/:id | API key | Delete email and stored attachments |
| PUT | /v1/webhook | API key | Configure webhook URL |
| PUT | /v1/settings | API key | Update TTL and display name |
| GET | /v1/emails/:id/attachments/:filename | API key | Download encrypted attachment |
| PUT | /v1/emails/:id/move | API key | Move email to a folder |

## Attachments

- Outbound API attachments use `{ filename, content_type, content }`, where `content` is Base64.
- The CLI accepts repeatable `--attach <path>` flags.
- Outbound messages accept up to 20 attachments and 35 MiB across subject, body, and Base64 attachment content, keeping the provider request below Resend's 40 MB encoded email limit.
- Outbound attachments must be encrypted and uploaded before calling Resend. If validation, encryption, or storage fails, do not send the email.
- If Resend rejects an email, remove any encrypted objects uploaded for that attempt.
- `GCS_BUCKET` is required for outbound attachments so the sent-folder copy remains complete and decryptable.

## Crypto

- Each email uses a random AES-256 key for subject and body encryption.
- Subject and body nonces are `0x01` and `0x02`.
- Attachments use separate random AES-256 keys wrapped with the agent RSA public key and nonce indexes starting at `0x03`.
- Registration stores only the public key. The private key is generated and retained by the CLI.
- API keys are indexed by prefix and stored as Argon2id hashes.
