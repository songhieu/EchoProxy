// Main entry point. Re-exports the proxy submodule so `@echoproxy/sdk` can be
// used without /proxy when only proxy mode is needed.
export * from "./proxy.js";
export {
  Client as IngestClient,
  expressMiddleware,
  type CaptureEvent,
  type IngestConfig,
} from "./ingest.js";
export { captureFetch, type CaptureFetch } from "./capture.js";
