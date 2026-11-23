import test from "node:test";
import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { copyFileSync, mkdirSync, mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { runInNewContext } from "node:vm";

test("TigerVNC paste sends UTF-8 after negotiation without waiting for a clipboard request", async (t) => {
  const directory = mkdtempSync(join(tmpdir(), "cage-vnc-clipboard-"));
  t.after(() => rmSync(directory, { recursive: true, force: true }));
  mkdirSync(join(directory, "core"));
  const rfbFile = join(directory, "core", "rfb.js");
  copyFileSync(new URL("../vendor/novnc/core/rfb.js", import.meta.url), rfbFile);
  const patch = fileURLToPath(new URL("../../internal/cage/novnc-patch.py", import.meta.url));
  // The guest can apply the patch again when syncing its components.
  execFileSync("python3", [patch, directory]);
  execFileSync("python3", [patch, directory]);
  const source = readFileSync(rfbFile, "utf8");
  const start = source.indexOf("    _cageQueueInput(action) {");
  const end = source.indexOf("    clipboardPasteFrom(text) {", start);
  assert.ok(start >= 0 && end > start);
  const sent = [];
  const RFB = { messages: {
    extendedClipboardNotify: () => sent.push("notify"),
    extendedClipboardProvide: (_socket, _formats, [text]) => sent.push(text),
    clientFence: (_socket, _flags, id) => sent.push(id),
  } };
  const Viewer = runInNewContext(`class Viewer extends EventTarget {${source.slice(start, end)}}; Viewer`, {
    RFB, EventTarget, setTimeout, clearTimeout,
    extendedClipboardFormatText: 1, extendedClipboardActionNotify: 8, extendedClipboardActionProvide: 16,
  });
  const client = Object.assign(new Viewer(), {
    _rfbConnectionState: "connected",
    _clipboardServerCapabilitiesFormats: {},
    _clipboardServerCapabilitiesActions: {},
  });
  client.cageUseTigerVNCClipboard();

  const paste = client.cagePasteText("中文🙂");
  await Promise.resolve();
  assert.deepEqual(sent, []);
  client._supportsFence = true;
  client.dispatchEvent(new Event("cage-clipboard-capability"));
  await Promise.resolve();
  assert.deepEqual(sent, []);
  client._clipboardServerCapabilitiesFormats = { 1: true };
  client._clipboardServerCapabilitiesActions = { 8: true, 16: true };
  client.dispatchEvent(new Event("cage-clipboard-capability"));
  await Promise.resolve();
  assert.deepEqual(sent, ["notify", "中文🙂", "cage-paste-1"]);
  assert.equal(client._cagePasteJob.phase, "fence");
  clearTimeout(client._cagePasteJob.timer);
  client._cagePasteJob.resolve();
  await paste;
  await assert.rejects(client.cagePasteText("old tab", () => false), /粘贴位置已切换/);
  assert.equal(sent.length, 3);
});
