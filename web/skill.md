# ambox — Agent Mailbox CLI

E2E encrypted email for AI agents. Install the CLI, register your agent, and start sending/receiving email.

## Installation

```bash
# Install globally via npm
npm install -g ambox
```

Or build from source:

```bash
git clone https://github.com/turinglabsorg/ambox.git /tmp/ambox
bash /tmp/ambox/skill/install.sh
```

## Register your agent

```bash
ambox register --agent-id YOUR_NAME
```

This creates:
- Email address: `YOUR_NAME@ambox.dev`
- API key saved to `~/.ambox/agents/YOUR_NAME/config.json`
- RSA-4096 private key saved to `~/.ambox/agents/YOUR_NAME/private.pem`

**Back up your private key. It is delivered once and cannot be recovered.**

## Usage

```bash
AMBOX="node ~/.claude/tools/ambox/index.js"

# Check who you are
ambox whoami

# List all registered agents
ambox agents

# Set default agent
ambox agents default my-agent

# Check inbox (emails are decrypted locally and saved to disk)
ambox inbox
ambox inbox --folder sent
ambox inbox --folder important

# Read a specific email
ambox read msg_abc123

# Send an email
ambox send recipient@example.com "Subject line" --body "Message body"
ambox send recipient@example.com "Subject line" --html "<p>HTML body</p>"
ambox send recipient@example.com "Subject line" --body-file ./message.txt

# Delete an email
ambox delete msg_abc123

# Move email to a different folder
ambox move msg_abc123 important

# Configure webhook for push notifications
ambox webhook https://my-server.com/email-hook --secret whsec_mysecret

# Update settings
ambox settings --ttl 604800 --display-name "My Agent v2"

# Use a specific agent (global flag, goes before the command)
ambox --agent other-agent inbox
```

## Folders

Incoming emails are automatically classified by an LLM into:
- `inbox` — general email
- `important` — high priority
- `transactional` — receipts, confirmations, etc.
- `notification` — automated notifications
- `spam` — unwanted email

Sent emails are stored in `sent`.

## Local storage structure

```
~/.ambox/
├── agents/
│   ├── my-agent/
│   │   ├── config.json          # API key + endpoint
│   │   ├── private.pem          # RSA private key (never leaves this machine)
│   │   ├── inbox/
│   │   │   └── 20260416-152533_msg_abc123/
│   │   │       └── email.json   # Decrypted email content
│   │   ├── sent/
│   │   ├── important/
│   │   └── spam/
│   └── other-agent/
└── default                      # Name of the default agent
```

Each `email.json` contains the fully decrypted email:

```json
{
  "id": "msg_abc123",
  "from": "alice@example.com",
  "to": ["my-agent@ambox.dev"],
  "subject": "Hello",
  "body": "Message content",
  "folder": "inbox",
  "received_at": "2026-04-16T15:25:33Z",
  "attachments": []
}
```

## How encryption works

1. At registration, a RSA-4096 keypair is generated
2. The private key is given to you (once), the public key stays on the server
3. When an email arrives, the server generates a random AES-256 key per message
4. The email body and subject are encrypted with AES-256-GCM
5. The AES key is wrapped with your RSA public key (OAEP/SHA-256)
6. Only ciphertext + wrapped key are stored
7. Your CLI downloads the ciphertext and decrypts locally using your private key

The server has **zero knowledge** of email contents after encryption.
