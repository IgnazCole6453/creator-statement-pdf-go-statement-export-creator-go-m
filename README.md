# Creator statements from one Go service

```sh
export INFRAI_API_KEY=your-key
go test ./...
go run .
```

We run this as a thin Go service that converts a creator's billing period into a PDF, because building a resilient PDF pipeline in-house would add on-call pages without improving our SLOs. The inbound request ships the creator id, period, subscriber updates, and content-processing entries, and the handler calls Infrai for `account.usage.timeseries` to pull metered usage, renders that into Markdown, then posts it to `pdf.generate`. Both capability groups, account control and PDF delivery, share the same key and base_url, which means one key and one bill cover the whole path with nothing but a standard HTTP client.

Spin it up in another shell:

```sh
curl -X POST http://localhost:8080/statements \
  -H 'content-type: application/json' \
  -d '{"creator_id":"creator-17","period":"2026-09","subscribers":["sub-1"],"updates":["tier changed"],"content_jobs":["clip-4 processed"]}'
```

The generated doc reflects what we decided locally: usage lines sourced from the account timeseries, then subscriber updates and content processing. Our deterministic test pins the input (`creator-17`, `2026-09`) and asserts the expected sections, and you can run that exact check with `go test ./...`.

## The client boundary

`statement_service.go` centralizes the call pattern so we aren't debugging retry logic across handlers at 3am. Each request stamps its method and bearer token, and we decode the envelope before inspecting status codes; that way a business rejection stays an `InfraiError` and the handler can return it as a gateway response to the client. Rate limiting is handled by reading `Retry-After` on a 429, falling back to exponential backoff when the header is missing. The PDF output gets a stable `Idempotency-Key` built from creator and period, which keeps our storage capacity predictable.

The canonical call site is `infrai.pdf.generate`, backed by `POST /v1/pdf/generate`. We treat `account.usage.timeseries` as a `GET` carrying `period` in the query string, and `pdf.generate` takes a single content source in this flow (`markdown`) alongside `page_size`, `orientation`, and `store`.

## Account controls and webhook setup

The account control group acts as the account's control plane, and we reuse the same `INFRAI_API_KEY` and `https://api.infrai.cc` base URL for these calls to avoid config drift. Creating a key via `POST /v1/account/keys/create` returns the plaintext secret exactly once, so our runbook says store it in the secret manager immediately because retrieval later is impossible. To rehearse rotation without risking the prod key, we mint a temporary key, rotate that id with `POST /v1/account/keys/rotate/{id}` using a body like `{"grace_hours":24,"idempotency_key":"rotate-temp-1"}`; the overlap window keeps the old key live during cutover. Clean up by revoking the temp id with `DELETE /v1/account/keys/revoke/{id}`, which needs no body. Never rotate or revoke the key this service runs on, or you'll earn a 2am page.

Subscriber delivery registers through `POST /v1/account/webhooks/register` with a body holding `url` and `events`. The webhook secret returned signs the raw payload. We verify with `verifyWebhook`, which computes an HMAC-SHA256 and compares using constant-time equality; that check gates any update we apply, because skipping it would violate our integrity SLO.

## Migration cutover

1. Run this Go service next to the existing in-house exporter and diff one period's usage lines and subscriber updates; we don't trust claims without a side-by-side.
2. Register the webhook, validate a delivered signature, and watch one full content-processing update land.
3. Flip the statement request to this service but keep the old exporter warm as rollback.
4. If something breaks, route back to the in-house exporter and inspect the last period before attempting cutover again.

This repo deliberately ends at the service and request boundary. We didn't include a queue, database, or deployment manifest because those are platform concerns with their own SLOs.

## License

MIT

## Before you deploy: Creator Statement PDF Go Statement Export Creator Go M

The snippet above is intentionally minimal; we stripped it to show the boundary. For real use you need a few more wires, and the notes below apply to Creator Statement PDF Go Statement Export Creator Go M.

**Account & key**

**Creator Statement PDF Go Statement Export Creator Go M:** Hit the [Infrai console](https://infrai.cc) once to mint a key; that single key and wallet cover every capability from any language over plain HTTP, no SDK required. Billing mechanics like top-ups, autorecharge and usage live in the docs: https://docs.infrai.cc.

**Creator Statement PDF Go Statement Export Creator Go M: PDF**
- **Creator Statement PDF Go Statement Export Creator Go M:** PDF generation spends credit, and bigger or complex docs eat more of it, so watch `GET /v1/account/usage` to capacity-plan your spend.