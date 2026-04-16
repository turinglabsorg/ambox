#!/usr/bin/env node

import { readFileSync, writeFileSync, mkdirSync, existsSync, readdirSync } from "fs";
import { homedir } from "os";
import { join } from "path";
import { generateKeyPairSync, createPrivateKey, privateDecrypt, createDecipheriv, constants } from "crypto";

const AMBOX_DIR = join(homedir(), ".ambox");
const AGENTS_DIR = join(AMBOX_DIR, "agents");
const DEFAULT_FILE = join(AMBOX_DIR, "default");

function getAgentDir(agentName) {
  return join(AGENTS_DIR, agentName);
}

function getDefaultAgent() {
  if (existsSync(DEFAULT_FILE)) {
    return readFileSync(DEFAULT_FILE, "utf-8").trim();
  }
  if (existsSync(AGENTS_DIR)) {
    const agents = readdirSync(AGENTS_DIR).filter((f) =>
      existsSync(join(AGENTS_DIR, f, "config.json"))
    );
    if (agents.length === 1) return agents[0];
    if (agents.length > 1) {
      console.error("Multiple agents found. Set a default with: ambox agents default <name>");
      console.error("Available: " + agents.join(", "));
      process.exit(1);
    }
  }
  const oldConfig = join(AMBOX_DIR, "config.json");
  if (existsSync(oldConfig)) {
    const cfg = JSON.parse(readFileSync(oldConfig, "utf-8"));
    migrateOldLayout(cfg.agentId);
    return cfg.agentId;
  }
  console.error("No agents registered. Run: ambox register --agent-id <name>");
  process.exit(1);
}

function migrateOldLayout(agentId) {
  const oldConfig = join(AMBOX_DIR, "config.json");
  const oldKey = join(AMBOX_DIR, "private.pem");
  if (!existsSync(oldConfig)) return;
  const dir = getAgentDir(agentId);
  mkdirSync(dir, { recursive: true, mode: 0o700 });
  if (!existsSync(join(dir, "config.json"))) {
    writeFileSync(join(dir, "config.json"), readFileSync(oldConfig), { mode: 0o600 });
  }
  if (existsSync(oldKey) && !existsSync(join(dir, "private.pem"))) {
    writeFileSync(join(dir, "private.pem"), readFileSync(oldKey), { mode: 0o600 });
  }
  setDefault(agentId);
}

function setDefault(agentName) {
  mkdirSync(AMBOX_DIR, { recursive: true, mode: 0o700 });
  writeFileSync(DEFAULT_FILE, agentName);
}

function resolveAgent(args) {
  return args["--agent"] || getDefaultAgent();
}

function loadConfig(agentName) {
  const configPath = join(getAgentDir(agentName), "config.json");
  if (!existsSync(configPath)) {
    console.error(`Agent "${agentName}" not found. Run: ambox register --agent-id ${agentName}`);
    process.exit(1);
  }
  return JSON.parse(readFileSync(configPath, "utf-8"));
}

function loadPrivateKey(agentName) {
  const keyPath = join(getAgentDir(agentName), "private.pem");
  if (!existsSync(keyPath)) {
    console.error(`Private key not found for agent "${agentName}" at ${keyPath}`);
    process.exit(1);
  }
  return createPrivateKey(readFileSync(keyPath, "utf-8"));
}

// --- Local storage helpers ---

function tsPrefix(isoDate) {
  return new Date(isoDate).toISOString().replace(/[-:]/g, "").replace("T", "-").slice(0, 15);
}

function emailDirName(email) {
  return `${tsPrefix(email.received_at)}_${email.id}`;
}

function getFolderDir(agentName, folder) {
  const dir = join(getAgentDir(agentName), folder);
  mkdirSync(dir, { recursive: true });
  return dir;
}

function saveEmailLocally(agentName, email, decrypted) {
  const folderDir = getFolderDir(agentName, email.folder);
  const msgDir = join(folderDir, emailDirName(email));
  mkdirSync(msgDir, { recursive: true });

  const data = {
    id: email.id,
    from: email.from,
    to: email.to,
    subject: decrypted.subject,
    body: decrypted.body,
    folder: email.folder,
    received_at: email.received_at,
    attachments: (email.attachments || []).map((a) => ({
      filename: a.filename,
      content_type: a.content_type,
      size_bytes: a.size_bytes,
    })),
  };

  writeFileSync(join(msgDir, "email.json"), JSON.stringify(data, null, 2));
  return msgDir;
}

function findLocalEmail(agentName, msgId) {
  const agentDir = getAgentDir(agentName);
  const folders = ["inbox", "sent", "important", "transactional", "notification", "spam"];
  for (const folder of folders) {
    const folderDir = join(agentDir, folder);
    if (!existsSync(folderDir)) continue;
    for (const entry of readdirSync(folderDir)) {
      if (entry.includes(msgId)) {
        const emailPath = join(folderDir, entry, "email.json");
        if (existsSync(emailPath)) {
          return JSON.parse(readFileSync(emailPath, "utf-8"));
        }
      }
    }
  }
  return null;
}

function findLocalEmailDir(agentName, msgId) {
  const agentDir = getAgentDir(agentName);
  const folders = ["inbox", "sent", "important", "transactional", "notification", "spam"];
  for (const folder of folders) {
    const folderDir = join(agentDir, folder);
    if (!existsSync(folderDir)) continue;
    for (const entry of readdirSync(folderDir)) {
      if (entry.includes(msgId)) {
        return join(folderDir, entry);
      }
    }
  }
  return null;
}

// --- Crypto ---

function decryptWrappedKey(privateKey, wrappedKeyB64) {
  const wrappedKey = Buffer.from(wrappedKeyB64, "base64");
  return privateDecrypt(
    { key: privateKey, padding: constants.RSA_PKCS1_OAEP_PADDING, oaepHash: "sha256" },
    wrappedKey
  );
}

function decryptAesGcm(aesKey, ciphertextB64, nonceIndex) {
  const ciphertext = Buffer.from(ciphertextB64, "base64");
  const nonce = Buffer.alloc(12);
  nonce[11] = nonceIndex;
  const authTagLen = 16;
  const encrypted = ciphertext.subarray(0, ciphertext.length - authTagLen);
  const authTag = ciphertext.subarray(ciphertext.length - authTagLen);
  const decipher = createDecipheriv("aes-256-gcm", aesKey, nonce);
  decipher.setAuthTag(authTag);
  return Buffer.concat([decipher.update(encrypted), decipher.final()]).toString("utf-8");
}

function decryptEmail(privateKey, email) {
  const aesKey = decryptWrappedKey(privateKey, email.wrapped_key);
  const subject = decryptAesGcm(aesKey, email.subject_encrypted, 1);
  const body = decryptAesGcm(aesKey, email.body_encrypted, 2);
  return { subject, body };
}

// --- API ---

async function api(config, method, path, body) {
  const url = config.endpoint + path;
  const opts = {
    method,
    headers: {
      Authorization: `Bearer ${config.apiKey}`,
      "Content-Type": "application/json",
    },
  };
  if (body) opts.body = JSON.stringify(body);
  const resp = await fetch(url, opts);
  if (resp.status === 204) return null;
  const data = await resp.json();
  if (!resp.ok) {
    console.error(`API error ${resp.status}: ${data.error || JSON.stringify(data)}`);
    process.exit(1);
  }
  return data;
}

// --- Commands ---

async function cmdRegister(args) {
  const endpoint = args["--endpoint"] || "https://ambox.dev";

  console.log("Generating RSA-4096 keypair locally...");
  const { publicKey, privateKey } = generateKeyPairSync("rsa", {
    modulusLength: 4096,
    publicKeyEncoding: { type: "spki", format: "pem" },
    privateKeyEncoding: { type: "pkcs8", format: "pem" },
  });

  const body = { public_key_pem: publicKey };
  if (args["--agent-id"]) body.agent_id = args["--agent-id"];
  if (args["--display-name"]) body.display_name = args["--display-name"];

  const resp = await fetch(endpoint + "/v1/register", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  const data = await resp.json();
  if (!resp.ok) {
    console.error("Registration failed: " + data.error);
    process.exit(1);
  }

  const agentDir = getAgentDir(data.agent_id);
  mkdirSync(agentDir, { recursive: true, mode: 0o700 });
  for (const f of ["inbox", "sent", "important", "transactional", "notification", "spam"]) {
    mkdirSync(join(agentDir, f), { recursive: true });
  }

  writeFileSync(join(agentDir, "private.pem"), privateKey, { mode: 0o600 });

  writeFileSync(join(agentDir, "config.json"), JSON.stringify({
    apiKey: data.api_key,
    agentId: data.agent_id,
    email: data.email,
    endpoint,
  }, null, 2), { mode: 0o600 });

  const agents = existsSync(AGENTS_DIR) ? readdirSync(AGENTS_DIR) : [];
  if (agents.length <= 1 || args["--default"]) {
    setDefault(data.agent_id);
  }

  console.log("Registered successfully!");
  console.log("Agent ID:     " + data.agent_id);
  console.log("Email:        " + data.email);
  console.log("API Key:      " + data.api_key);
  console.log("Agent dir:    " + agentDir);
  console.log("\nIMPORTANT: Back up your private key. It cannot be recovered.");
}

async function cmdAgents(args) {
  const sub = args._[0];

  if (sub === "default" && args._[1]) {
    const name = args._[1];
    if (!existsSync(join(getAgentDir(name), "config.json"))) {
      console.error('Agent "' + name + '" not found.');
      process.exit(1);
    }
    setDefault(name);
    console.log("Default agent set to: " + name);
    return;
  }

  if (!existsSync(AGENTS_DIR)) {
    console.log("No agents registered.");
    return;
  }

  const currentDefault = existsSync(DEFAULT_FILE) ? readFileSync(DEFAULT_FILE, "utf-8").trim() : "";
  const agents = readdirSync(AGENTS_DIR).filter((f) =>
    existsSync(join(AGENTS_DIR, f, "config.json"))
  );

  if (agents.length === 0) {
    console.log("No agents registered.");
    return;
  }

  for (const name of agents) {
    const cfg = JSON.parse(readFileSync(join(AGENTS_DIR, name, "config.json"), "utf-8"));
    const isDefault = name === currentDefault ? " (default)" : "";
    console.log("  " + name + isDefault + " — " + cfg.email + " [" + cfg.endpoint + "]");
  }
}

async function cmdWhoami(args) {
  const agentName = resolveAgent(args);
  const config = loadConfig(agentName);
  console.log("Agent ID:  " + config.agentId);
  console.log("Email:     " + config.email);
  console.log("Endpoint:  " + config.endpoint);
  console.log("Agent dir: " + getAgentDir(agentName));
}

async function cmdInbox(args) {
  const agentName = resolveAgent(args);
  const config = loadConfig(agentName);
  const privateKey = loadPrivateKey(agentName);

  const folder = args["--folder"] || "inbox";
  const params = new URLSearchParams();
  params.set("folder", folder);
  if (args["--limit"]) params.set("limit", args["--limit"]);
  if (args["--since"]) params.set("since", args["--since"]);
  if (args["--cursor"]) params.set("cursor", args["--cursor"]);

  const data = await api(config, "GET", "/v1/inbox?" + params.toString());

  if (!data.emails || data.emails.length === 0) {
    console.log("No emails in " + folder + ".");
    return;
  }

  for (const email of data.emails) {
    try {
      const dec = decryptEmail(privateKey, email);
      saveEmailLocally(agentName, email, dec);
      const date = new Date(email.received_at).toLocaleString();
      const attachFlag = email.attachments?.length ? " [" + email.attachments.length + " att]" : "";
      console.log("[" + email.id + "] " + date + " | From: " + email.from + " | " + dec.subject + attachFlag);
    } catch {
      console.log("[" + email.id + "] " + email.received_at + " | From: " + email.from + " | [decrypt failed]");
    }
  }

  if (data.has_more) {
    const lastId = data.emails[data.emails.length - 1].id;
    console.log("\nMore emails available. Use --cursor " + lastId);
  }
}

async function cmdRead(args) {
  const agentName = resolveAgent(args);
  const msgId = args._[0];
  if (!msgId) {
    console.error("Usage: ambox read <message-id>");
    process.exit(1);
  }

  // check local first
  const local = findLocalEmail(agentName, msgId);
  if (local) {
    printEmail(local);
    return;
  }

  // fetch from server
  const config = loadConfig(agentName);
  const privateKey = loadPrivateKey(agentName);

  for (const folder of ["inbox", "sent", "important", "transactional", "notification", "spam"]) {
    const data = await api(config, "GET", "/v1/inbox?folder=" + folder + "&limit=100");
    const email = data.emails?.find((e) => e.id === msgId);
    if (email) {
      const dec = decryptEmail(privateKey, email);
      saveEmailLocally(agentName, email, dec);
      printEmail({ ...email, subject: dec.subject, body: dec.body });
      return;
    }
  }
  console.error("Email not found.");
  process.exit(1);
}

function printEmail(e) {
  console.log("ID:      " + e.id);
  console.log("From:    " + e.from);
  console.log("To:      " + (Array.isArray(e.to) ? e.to.join(", ") : e.to));
  console.log("Date:    " + new Date(e.received_at).toLocaleString());
  console.log("Folder:  " + e.folder);
  console.log("Subject: " + e.subject);
  console.log("---");
  console.log(e.body);
  if (e.attachments?.length) {
    console.log("\nAttachments:");
    for (const att of e.attachments) {
      console.log("  - " + att.filename + " (" + (att.content_type || "") + ", " + (att.size_bytes || 0) + " bytes)");
    }
  }
}

async function cmdSend(args) {
  const agentName = resolveAgent(args);
  const to = args._[0];
  const subject = args._[1];
  if (!to || !subject) {
    console.error("Usage: ambox send <to> <subject> [--body text] [--html html] [--body-file path]");
    process.exit(1);
  }

  const config = loadConfig(agentName);
  const body = { to: [to], subject };

  if (args["--body"]) body.body_text = args["--body"];
  if (args["--html"]) body.body_html = args["--html"];
  if (args["--body-file"]) body.body_text = readFileSync(args["--body-file"], "utf-8");
  if (!body.body_text && !body.body_html) body.body_text = "";

  const data = await api(config, "POST", "/v1/send", body);
  console.log("Email sent!");
  console.log("Message ID: " + data.message_id);
  console.log("Resend ID:  " + data.resend_id);
}

async function cmdDelete(args) {
  const agentName = resolveAgent(args);
  const msgId = args._[0];
  if (!msgId) {
    console.error("Usage: ambox delete <message-id>");
    process.exit(1);
  }
  const config = loadConfig(agentName);
  await api(config, "DELETE", "/v1/emails/" + msgId);
  console.log("Email " + msgId + " deleted.");
}

async function cmdWebhook(args) {
  const agentName = resolveAgent(args);
  const url = args._[0];
  if (!url) {
    console.error("Usage: ambox webhook <url> [--secret whsec_...]");
    process.exit(1);
  }
  const config = loadConfig(agentName);
  const body = { url };
  if (args["--secret"]) body.secret = args["--secret"];
  await api(config, "PUT", "/v1/webhook", body);
  console.log("Webhook configured: " + url);
}

async function cmdSettings(args) {
  const agentName = resolveAgent(args);
  const config = loadConfig(agentName);
  const body = {};
  if (args["--ttl"]) body.ttl_seconds = parseInt(args["--ttl"]);
  if (args["--display-name"]) body.display_name = args["--display-name"];
  if (Object.keys(body).length === 0) {
    console.error("Usage: ambox settings [--ttl seconds] [--display-name name]");
    process.exit(1);
  }
  await api(config, "PUT", "/v1/settings", body);
  console.log("Settings updated.");
}

async function cmdMove(args) {
  const agentName = resolveAgent(args);
  const msgId = args._[0];
  const folder = args._[1];
  if (!msgId || !folder) {
    console.error("Usage: ambox move <message-id> <folder>");
    process.exit(1);
  }
  const config = loadConfig(agentName);
  await api(config, "PUT", "/v1/emails/" + msgId + "/move", { folder });
  console.log("Email " + msgId + " moved to " + folder + ".");
}

async function cmdDownload(args) {
  const agentName = resolveAgent(args);
  const msgId = args._[0];
  const filename = args._[1];
  if (!msgId || !filename) {
    console.error("Usage: ambox download <message-id> <filename> [--output path]");
    process.exit(1);
  }

  const config = loadConfig(agentName);
  const privateKey = loadPrivateKey(agentName);

  const url = config.endpoint + "/v1/emails/" + msgId + "/attachments/" + encodeURIComponent(filename);
  const resp = await fetch(url, {
    headers: { Authorization: "Bearer " + config.apiKey },
  });

  if (!resp.ok) {
    const data = await resp.json();
    console.error("Download failed: " + data.error);
    process.exit(1);
  }

  const wrappedKeyB64 = resp.headers.get("x-ambox-wrapped-key");
  const nonceIndex = parseInt(resp.headers.get("x-ambox-nonce-index"));
  const encryptedData = Buffer.from(await resp.arrayBuffer());

  const aesKey = decryptWrappedKey(privateKey, wrappedKeyB64);
  const nonce = Buffer.alloc(12);
  nonce[11] = nonceIndex;

  const authTagLen = 16;
  const encrypted = encryptedData.subarray(0, encryptedData.length - authTagLen);
  const authTag = encryptedData.subarray(encryptedData.length - authTagLen);
  const decipher = createDecipheriv("aes-256-gcm", aesKey, nonce);
  decipher.setAuthTag(authTag);
  const decrypted = Buffer.concat([decipher.update(encrypted), decipher.final()]);

  // save to email dir if exists, otherwise to --output or cwd
  const emailDir = findLocalEmailDir(agentName, msgId);
  const outputPath = args["--output"] || (emailDir ? join(emailDir, filename) : filename);
  writeFileSync(outputPath, decrypted);
  console.log("Attachment saved to: " + outputPath + " (" + decrypted.length + " bytes)");
}

// --- Arg parsing ---

function parseArgs(argv) {
  const args = { _: [] };
  for (let i = 0; i < argv.length; i++) {
    if (argv[i].startsWith("--")) {
      const key = argv[i];
      if (i + 1 < argv.length && !argv[i + 1].startsWith("--")) {
        args[key] = argv[++i];
      } else {
        args[key] = true;
      }
    } else {
      args._.push(argv[i]);
    }
  }
  return args;
}

const commands = {
  register: cmdRegister,
  agents: cmdAgents,
  whoami: cmdWhoami,
  inbox: cmdInbox,
  read: cmdRead,
  send: cmdSend,
  delete: cmdDelete,
  webhook: cmdWebhook,
  settings: cmdSettings,
  move: cmdMove,
  download: cmdDownload,
};

// Extract global flags before command
const rawArgs = process.argv.slice(2);
let command;
const rest = [];
const globalFlags = {};
let i = 0;
while (i < rawArgs.length) {
  if (!command && rawArgs[i] === "--agent" && i + 1 < rawArgs.length) {
    globalFlags["--agent"] = rawArgs[++i];
    i++;
  } else if (!command && !rawArgs[i].startsWith("--")) {
    command = rawArgs[i];
    i++;
  } else {
    rest.push(rawArgs[i]);
    i++;
  }
}

if (!command || !commands[command]) {
  console.log("ambox — Agent Mailbox CLI");
  console.log("");
  console.log("Commands:");
  console.log("  register    Register a new agent");
  console.log("  agents      List agents / set default");
  console.log("  whoami      Show current agent info");
  console.log("  inbox       List emails (decrypted)");
  console.log("  read        Read a specific email");
  console.log("  send        Send an email");
  console.log("  delete      Delete an email");
  console.log("  move        Move email to folder");
  console.log("  webhook     Configure webhook URL");
  console.log("  settings    Update agent settings");
  console.log("  download    Download attachment");
  console.log("");
  console.log("Global flags:");
  console.log("  --agent <name>    Use a specific agent (default: auto)");
  console.log("");
  console.log("Usage: ambox <command> [args]");
  process.exit(command ? 1 : 0);
}

const args = { ...globalFlags, ...parseArgs(rest) };
commands[command](args).catch((err) => {
  console.error("Error: " + err.message);
  process.exit(1);
});
