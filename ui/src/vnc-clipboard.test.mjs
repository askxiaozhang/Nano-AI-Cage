import test from "node:test";
import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { copyFileSync, mkdirSync, mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { runInNewContext } from "node:vm";

function patchedRfb(t) {
  const directory = mkdtempSync(join(tmpdir(), "cage-vnc-clipboard-"));
  t.after(() => rmSync(directory, { recursive: true, force: true }));
  mkdirSync(join(directory, "core"));
  const rfbFile = join(directory, "core", "rfb.js");
  copyFileSync(new URL("../vendor/novnc/core/rfb.js", import.meta.url), rfbFile);
  const patch = fileURLToPath(new URL("../../internal/cage/novnc-patch.py", import.meta.url));
  // The guest can apply the patch again when syncing its components.
  execFileSync("python3", [patch, directory]);
  execFileSync("python3", [patch, directory]);
  return readFileSync(rfbFile, "utf8");
}

test("TigerVNC paste sends UTF-8 after negotiation without waiting for a clipboard request", async (t) => {
  const source = patchedRfb(t);
  const start = source.indexOf("    _cageQueueInput(");
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

test("a failed punctuation commit still releases a held Shift and skips queued Enter", async (t) => {
  const source = patchedRfb(t);
  const queueStart = source.indexOf("    _cageQueueInput(");
  const queueEnd = source.indexOf("    cageRunInput(", queueStart);
  const keyStart = source.indexOf("    sendKey(keysym, code, down) {");
  const keyEnd = source.indexOf("    _cageSendKey(keysym, code, down) {", keyStart);
  assert.ok(queueStart >= 0 && queueEnd > queueStart && keyStart >= 0 && keyEnd > keyStart);
  const Viewer = runInNewContext(`class Viewer {
    ${source.slice(queueStart, queueEnd)}
    ${source.slice(keyStart, keyEnd)}
  }; Viewer`);
  const client = new Viewer();
  const sent = [];
  client._cageModifiers = new Map();
  client._cageSendKey = (keysym, code, down) => {
    sent.push([code, down]);
    if (code === "ShiftLeft") {
      if (down) client._cageModifiers.set(code, keysym);
      else client._cageModifiers.delete(code);
    }
  };

  client.sendKey(0xffe1, "ShiftLeft", true);
  const failure = new Error("punctuation transfer failed");
  const commit = client._cageQueueInput(() => Promise.reject(failure));
  client.sendKey(0xffe1, "ShiftLeft", false);
  client.sendKey(0xff0d, "Enter", true);
  client.sendKey(0xff0d, "Enter", false);

  await assert.rejects(client._cageInputChain, failure);
  await assert.rejects(commit, failure);
  assert.deepEqual(sent, [["ShiftLeft", true], ["ShiftLeft", false]]);
  assert.equal(client._cageModifiers.size, 0);
  client.sendKey(0x61, "KeyA", true);
  assert.deepEqual(sent.at(-1), ["KeyA", true]);
});
