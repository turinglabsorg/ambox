---
name: ambox
description: "Agent Mailbox — send, receive, and manage E2E encrypted emails via ambox.dev. Use when the agent needs email capabilities: check inbox, send messages, or manage its mailbox."
allowed-tools: Bash, Read, Write
argument-hint: "[command] [args]"
---

# Agent Mailbox (ambox)

You have access to the `ambox` CLI tool for managing your encrypted email mailbox at ambox.dev.

## First-time setup

If the CLI is not installed yet, run:

```bash
npm install -g ambox
ambox register --agent-id YOUR_NAME
```

This gives you an email address `YOUR_NAME@ambox.dev` and saves your private key locally. The private key is delivered once and never stored on the server.

## Commands

```bash

# Identity
ambox whoami                                     # Show current agent info
ambox agents                                     # List all registered agents
ambox agents default <name>                      # Set default agent

# Read emails (decrypted locally)
ambox inbox                                      # Check inbox
ambox inbox --folder sent                        # Check sent folder
ambox inbox --folder important                   # Check important folder
ambox read <message-id>                          # Read specific email

# Send emails
ambox send <to> <subject> --body "text"          # Send with text body
ambox send <to> <subject> --html "<p>html</p>"   # Send with HTML body
ambox send <to> <subject> --body-file path       # Send with body from file

# Manage
ambox delete <message-id>                        # Delete email
ambox move <message-id> <folder>                 # Move to folder
ambox webhook <url>                              # Set webhook for push notifications
ambox settings --ttl 604800                      # Set email TTL (seconds, 0=forever)

# Multi-agent (use --agent before the command)
ambox --agent other-agent inbox                  # Check another agent's inbox
```

## Folders

Incoming emails are auto-classified: `inbox`, `important`, `transactional`, `notification`, `spam`. Sent emails go to `sent`.

## Local storage

Emails are decrypted and saved to `~/.ambox/agents/{name}/{folder}/{timestamp}_{id}/email.json`. You can read them directly from disk without hitting the API.

## Important

- All emails are E2E encrypted. Decryption happens locally using your private key at `~/.ambox/agents/{name}/private.pem`.
- The private key is generated once at registration and **cannot be recovered**. Back it up.
- Config is at `~/.ambox/agents/{name}/config.json`.
