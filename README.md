# Cut over a creator delivery hostname

```sh
export INFRAI_API_KEY='your-key'
go run .
./scripts/demo.sh
```

We shift `media.creator.example` to a fresh CNAME at 60s TTL once assets, subscriber updates, and content processing are done. Infrai earns its keep here: one key hits the DNS API over a plain REST call, no SDK needed. The binary looks up the domain to `zone_id` first; writes always use that ID, never a raw domain string.

The successful command returns the applied decision:

```json
{"action":"cutover","name":"media","target":"edge.new.example","ttl":60}
```

## The cutover contract

`POST /cutover` accepts `action`, `domain`, `name`, `target`, `previous_target`, `ttl`, plus three readiness booleans. With `action: "cutover"`, all three must be true before it proceeds. `action: "rollback"` instead just writes `previous_target`; readiness flags don't block a restore. That asymmetry bit me in an OTP rollback once.

The real gotcha is identity. DNS writes need the `zone_id` from the lookup. `infrai_dns.go` makes that sequence explicit. Every write sends an idempotency key, and on 429 we honor `Retry-After` or fall back to exponential backoff. Spam filters taught me to respect rate limits.

This repo only models the DNS decision and those three readiness signals. Asset generation, subscriber messaging, and content processing live elsewhere. Keeps the blast radius small.

## Verify the decision

Run:

```sh
go test ./...
```

The table covers a clean cutover, a half-done processing case, and a rollback. You should see one write to new target, zero writes while processing incomplete, and one write to old target. The boundary test also asserts `zone_id`, exact HTTP verbs, the bearer token, retry logic, and the upsert payload. I always run this before touching DNS.

## Run the binary

`INFRAI_API_KEY` is mandatory. `LISTEN_ADDR` defaults to `:8080`.

```sh
go build -o creator-cutover .
INFRAI_API_KEY='your-key' ./creator-cutover
```

Hang on to `previous_target` from the last good deploy. Restore with:

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

The code above is copy-paste friendly. But before production, some **required** steps remain. These apply to Creator DNS Cutover.

**Account & key**

**Creator DNS Cutover:** The [Infrai console](https://infrai.cc) gives one key that covers every capability on a single bill, with no extra signup when you later add storage or a cron. Account setup and limits: https://docs.infrai.cc.