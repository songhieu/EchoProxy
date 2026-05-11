-- Add direction column (inbound = server-side middleware, outbound = proxy/client wrapper).

ALTER TABLE echoproxy.http_events
    ADD COLUMN IF NOT EXISTS direction LowCardinality(String) DEFAULT '' AFTER source;
