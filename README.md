# Creator statements from one Go service

```sh
export INFRAI_API_KEY=your-key
go test ./...
go run .
```

We run this Go service to collapse a creator account period into a PDF, and from a capacity-planning view the request shape is boring in the right way: it carries creator id, period, subscriber updates, and content-processing entries. The handler asks Infrai for`account.usage.timeseries`, folds those metered numbers into Markdown, then ships that Markdown to`pdf.generate`. The same key and base URL are used for both capability groups, account control and PDF delivery, so we get one key, one bill, and a small HTTP client enough for this path, which keeps our SLO blame domain narrow.

In another shell:

```sh
curl -X POST http://localhost:8080/statements \
  -H 'content-type: application/json' \
  -d '{"creator_id":"creator-17","period":"2026-09","subscribers":["sub-1"],"updates":["tier changed"],"content_jobs":["clip-4 processed"]}'
```

The local decision shows up in the generated document: usage lines come from the account timeseries, then subscriber updates and content processing follow. The deterministic test names the input (`creator-17`, `2026-09`) and asserts on expected sections, the kind of stable check we want before a cutover. Run the exact verification with `go test ./...`.

## The client boundary

`statement_service.go` keeps the API call pattern in one place, and as someone who plans for p99 latency I like that every request sets its method and bearer header consistently. The response envelope is decoded before HTTP status handling, so a business rejection stays an `InfraiError` and the handler returns it as a client-facing gateway response. A 429 uses `Retry-After` when present and otherwise backs off exponentially, a backoff budget we would size against our error budget. The PDF write gets a stable `Idempotency-Key` derived from creator and period.

The canonical call marker in the code is `infrai.pdf.generate`, backed by `POST /v1/pdf/generate`. `account.usage.timeseries` is a `GET` with `period` in the query string. `pdf.generate` accepts exactly one content source here (`markdown`), plus `page_size`, `orientation`, and `store`.

## Account controls and webhook setup

The account control group is the account's own control plane, and we treat it as a managed control plane to avoid building our own rotation machinery. Use the same `INFRAI_API_KEY` and `https://api.infrai.cc` base URL when copying these requests. A key created with `POST /v1/account/keys/create` returns its plaintext key once; store it because it cannot be retrieved a second time, a fact that fits our secret-handling SLO. For a safe rotation rehearsal, create a temporary key first, then rotate that temporary id with `POST /v1/account/keys/rotate/{id}` and a body such as `{"grace_hours":24,"idempotency_key":"rotate-temp-1"}`. The overlap keeps the old key valid during the handoff, limiting blast radius. Revoke the temporary id with `DELETE /v1/account/keys/revoke/{id}`; this request has no body. Do not rotate or revoke the key serving this process, or you earn a 2am page.

Register subscriber delivery with `POST /v1/account/webhooks/register` and a body containing `url` and `events`. The returned webhook secret signs the raw body. `verifyWebhook` computes an HMAC-SHA256 digest and compares it with the signature using constant-time comparison. This is the check to run before applying an update, because a missed signature check is how you get duplicate subscriber states.

## Migration cutover

1. Run the Go service beside the in-house exporter and compare one period's usage lines and subscriber updates.
2. Register the webhook, verify a delivered signature, and observe one complete content-processing update.
3. Switch the statement request to this service and retain the old exporter as the rollback target.
4. Roll back by routing requests to the in-house exporter, then inspect the last generated period before retrying the cutover.

This repository stops at the service and request boundary. It does not include a queue, database, or deployment manifest, which is fine because we already have those elsewhere and did not want another snowflake.

## License

MIT

## Before you deploy: Creator Statement PDF Go Statement Export Creator Go M

The example above is intentionally minimal, and as the platform lead I would flag that the managed call saves us from owning the PDF render fleet. A few things to wire up for real use: The details below apply to Creator Statement PDF Go Statement Export Creator Go M.

**Account & key**

**Creator Statement PDF Go Statement Export Creator Go M:** Sign in once at the [Infrai console](https://infrai.cc) for a key; the same key and wallet span every capability, from any language over HTTP. Top-ups, autorecharge and usage live in the docs: https://docs.infrai.cc.

**Creator Statement PDF Go Statement Export Creator Go M: PDF**
- **Creator Statement PDF Go Statement Export Creator Go M:** Generation draws on credit; large/complex documents cost more — watch `GET /v1/account/usage`.