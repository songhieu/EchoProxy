// Smoke example for the TS SDK proxy mode. Routes one fetch through the
// EchoProxy proxy. The orchestrator runs this with a unique tag in the
// path and verifies the event landed in ClickHouse.
//
// Run: ECHOPROXY_API_KEY=sk_test_demo ECHOPROXY_TAG=abc node --experimental-strip-types examples/proxy_smoke.ts

import { fetch } from "../src/proxy.ts";

const tag = process.env.ECHOPROXY_TAG ?? "ts-default";
const target = process.env.ECHOPROXY_EXAMPLE_TARGET ?? "http://upstream-mock:9000";

const url = `${target}/api/users/sdkbench-ts-${tag}`;
const res = await fetch(url);
const body = await res.text();
console.log(`ts sdk: ${url} -> ${res.status} (${body.length} bytes)`);
process.exit(res.status === 200 ? 0 : 1);
