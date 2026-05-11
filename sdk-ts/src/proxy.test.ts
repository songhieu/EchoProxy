import { test } from "node:test";
import assert from "node:assert/strict";
import http from "node:http";
import { configure, fetch as sidFetch, post } from "./proxy.ts";

function startMockProxy(): Promise<{ url: string; close: () => void; lastReq: () => http.IncomingMessage | null }> {
  return new Promise((resolve) => {
    let lastReq: http.IncomingMessage | null = null;
    const server = http.createServer((req, res) => {
      lastReq = req;
      let body = "";
      req.on("data", (c) => (body += c));
      req.on("end", () => {
        res.writeHead(200, { "Content-Type": "application/json" });
        res.end(JSON.stringify({ ok: true, echo: body }));
      });
    });
    server.listen(0, () => {
      const addr = server.address();
      const port = typeof addr === "object" && addr ? addr.port : 0;
      resolve({
        url: `http://127.0.0.1:${port}`,
        close: () => server.close(),
        lastReq: () => lastReq,
      });
    });
  });
}

test("fetch rewrites URL and adds headers", async () => {
  const proxy = await startMockProxy();
  configure("sk_test_abc", proxy.url);
  const res = await sidFetch("https://api.example.com/v1/users?x=1");
  assert.equal(res.status, 200);
  const last = proxy.lastReq()!;
  assert.equal(last.headers["x-echo-key"], "sk_test_abc");
  assert.equal(last.headers["x-echo-target"], "https://api.example.com");
  assert.equal(last.url, "/v1/users?x=1");
  proxy.close();
});

test("post serializes JSON and sets content-type", async () => {
  const proxy = await startMockProxy();
  configure("sk_test_abc", proxy.url);
  await post("https://api.example.com/v1/users", { name: "Alice" });
  const last = proxy.lastReq()!;
  assert.equal(last.headers["content-type"], "application/json");
  proxy.close();
});
