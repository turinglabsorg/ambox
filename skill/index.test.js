import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { mkdtempSync, mkdirSync, rmSync, writeFileSync } from "node:fs";
import { createServer } from "node:http";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";

const skillDir = dirname(fileURLToPath(import.meta.url));

function runCLI(args, env) {
  return new Promise((resolve, reject) => {
    const child = spawn(process.execPath, [join(skillDir, "index.js"), ...args], { env });
    let stdout = "";
    let stderr = "";
    child.stdout.on("data", (chunk) => { stdout += chunk; });
    child.stderr.on("data", (chunk) => { stderr += chunk; });
    child.on("error", reject);
    child.on("close", (code) => resolve({ code, stdout, stderr }));
  });
}

test("send includes every repeated --attach file", async () => {
  const home = mkdtempSync(join(tmpdir(), "ambox-cli-test-"));
  const agentDir = join(home, ".ambox", "agents", "test-agent");
  mkdirSync(agentDir, { recursive: true });
  writeFileSync(join(home, ".ambox", "default"), "test-agent");

  let requestBody;
  const server = createServer((request, response) => {
    let body = "";
    request.on("data", (chunk) => { body += chunk; });
    request.on("end", () => {
      requestBody = JSON.parse(body);
      response.writeHead(200, { "Content-Type": "application/json" });
      response.end(JSON.stringify({ message_id: "msg_test", resend_id: "resend_test" }));
    });
  });

  try {
    await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
    const address = server.address();
    const endpoint = `http://127.0.0.1:${address.port}`;
    writeFileSync(join(agentDir, "config.json"), JSON.stringify({
      apiKey: "test-key",
      agentId: "test-agent",
      email: "test-agent@ambox.dev",
      endpoint,
    }));

    const csvPath = join(home, "credentials.csv");
    const textPath = join(home, "notes.txt");
    writeFileSync(csvPath, "city,code\nIspica,123456\n");
    writeFileSync(textPath, "Private delivery notes");

    const result = await runCLI([
      "send",
      "recipient@example.com",
      "Mobility credentials",
      "--body",
      "Attached.",
      "--attach",
      csvPath,
      "--attach",
      textPath,
    ], { ...process.env, HOME: home });

    assert.equal(result.code, 0, result.stderr);
    assert.match(result.stdout, /Email sent!/);
    assert.equal(requestBody.attachments.length, 2);
    assert.deepEqual(requestBody.attachments.map((attachment) => attachment.filename), ["credentials.csv", "notes.txt"]);
    assert.deepEqual(requestBody.attachments.map((attachment) => attachment.content_type), ["text/csv", "text/plain"]);
    assert.equal(Buffer.from(requestBody.attachments[0].content, "base64").toString(), "city,code\nIspica,123456\n");
    assert.equal(Buffer.from(requestBody.attachments[1].content, "base64").toString(), "Private delivery notes");
  } finally {
    await new Promise((resolve) => server.close(resolve));
    rmSync(home, { recursive: true, force: true });
  }
});
