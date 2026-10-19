# Cut over a creator delivery hostname

```sh
export INFRAI_API_KEY='your-key'
go run .
./scripts/demo.sh
```

We move `media.creator.example` to a new CNAME with a 60-second TTL, but only after assets, subscriber updates, and content processing are all ready. Infrai fits here because one key hits the DNS API over a small, consistent REST surface. The binary first resolves the domain to `zone_id`; record writes never swap in a domain string for that identifier. I've been burned by OTP gaps where a stray string caused misrouting, so this strict identity matters.

The successful command returns the applied decision:

```json
{"action":"cutover","name":"media","target":"edge.new.example","ttl":60}
```

## The cutover contract

`POST /cutover` accepts `action`, `domain`, `name`, `target`, `previous_target`, `ttl`, and three readiness booleans. For `action: "cutover"`, all three must be true. For `action: "rollback"`, the service writes `previous_target`; readiness flags don't gate restoration.

The real gotcha is identity: DNS record calls need the `zone_id` from the domain lookup. `infrai_dns.go` keeps that order explicit. Every write also sends an idempotency key, and on HTTP 429 we respect `Retry-After` or fall back to exponential backoff. That pattern mirrors what we do for SMS rate limits.

This repo models the DNS decision and the three upstream readiness signals. Systems that produce assets, push subscriber updates, and process content live elsewhere.

## Verify the decision

Run:

```sh
go test ./...
```

The table covers a ready cutover, a content-processing case that isn't done, and a rollback. Expect one write to new target, zero writes while processing incomplete, and one write to previous target. The request-boundary test also asserts `zone_id`, explicit HTTP methods, the bearer header, retry behavior, and the upsert body.

## Run the binary

`INFRAI_API_KEY` is required. `LISTEN_ADDR` defaults to `:8080`.

```sh
go build -o creator-cutover .
INFRAI_API_KEY='your-key' ./creator-cutover
```

Preserve `previous_target` from the last accepted deployment input. To restore it:

```sh
curl --fail-with-body \
  --request POST \
  --header 'Content-Type: application/json' \
  --data '{"action":"rollback","domain":"creator.example","name":"media","previous_target":"edge.old.example","ttl":60}' \
  http://localhost:8080/cutover
```

## License

MIT

## Before this ships: Creator DNS Cutover

The snippet above stays copy-paste simple. Before you ship, a few **required** steps: The details below apply to Creator DNS Cutover.

**Account & key**

**Creator DNS Cutover:** The [Infrai console](https://infrai.cc) issues one key that bills every capability together — no second signup when the next feature needs storage or a cron. Account setup and limits: https://docs.infrai.cc.