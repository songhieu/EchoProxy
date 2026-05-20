// Smoke example for the TS SDK *capture mode*. The app makes the upstream
// call itself via captureFetch — the SDK ships the event to ingest-api with
// source = sdk-ts. The orchestrator runs this with a unique tag in the path
// and verifies the event landed in ClickHouse.
//
// Run: ECHOPROXY_API_KEY=sk_test_demo ECHOPROXY_TAG=abc \
//      node --experimental-strip-types examples/capture_smoke.ts

import { Client as IngestClient } from "../src/ingest.ts";
import { captureFetch } from "../src/capture.ts";

const tag = process.env.ECHOPROXY_TAG ?? "ts-default";
const target = process.env.ECHOPROXY_EXAMPLE_TARGET ?? "http://upstream-mock:9000";

const client = new IngestClient({ flushIntervalMs: 200 });
const fetch = captureFetch(client);

const url = `${target}/api/users/sdkbench-ts-capture-${tag}`;
const res = await fetch(url);
const body = await res.text();
console.log(`ts sdk (capture): ${url} -> ${res.status} (${body.length} bytes)`);

// Drain the SDK buffer before exit so the smoke event is shipped.
client.close();
await new Promise((r) => setTimeout(r, 500));
process.exit(res.status === 200 ? 0 : 1);
