# bdxfund

Private, non-custodial crowdfunding on [Beldex](https://beldex.io).

- **Donors stay private.** Beldex hides sender, receiver and amount on-chain (RingCT, stealth addresses), so the site never learns who donated.
- **Non-custodial.** The server only holds a **view key**: it sees incoming donations but cannot spend them. Funds go straight to the organiser's wallet.
- **Publicly verifiable.** Every page publishes the wallet address and view key. Anyone can restore a view-only wallet and recompute every total.
- **Instant.** Donations sent with Beldex **Flash** show up within seconds instead of after block confirmations.
- **One campaign = one subaddress**, so totals never mix between campaigns.

## How it works

```
donor wallet ──BDX──► campaign subaddress (organiser's wallet)
                               │
             view key only     ▼
bdxfund ◄── beldex-wallet-rpc (view-only wallet) ◄── Beldex node
   │
   └─► campaign page: progress, donation list (amount/status/time, no senders),
       address + view key for independent audit
```

Donation status:

| status | meaning | counts toward goal |
|---|---|---|
| `confirmed` | included in a block | yes |
| `flash` | approved by the Flash quorum, not yet mined | yes |
| `pending` | ordinary mempool tx, can still fail | no |

## Run (testnet)

1. Start `beldex-wallet-rpc` against a testnet node and open a **view-only** wallet
   (`generate_from_keys` with address + view key; see `../scripts/testnet-wallet.sh`).
2. Create a campaign — this allocates a fresh subaddress:
   ```
   go run . new -id rent -title "Pay the rent" -goal 500 -desc "Help our hackerspace stay open"
   ```
3. Serve:
   ```
   go run . serve -addr 127.0.0.1:8080 -network testnet
   ```
   JSON API: `GET /api/campaigns`, `GET /api/campaigns/{id}`.

## Status

MVP. Known limitations:
- All campaigns of one server share one wallet; publishing its view key reveals incoming amounts for all of them (by design: transparency of the organiser, privacy of donors). Run one wallet per organiser if that matters.
- Flash finality assumptions still to be documented with the Beldex team; large donations should be treated as final only after block confirmations.
- Storage is a single JSON file.

Built on [`beldex-go`](https://github.com/rebrudiy/beldex-go).
