# Official Balance Filters and Granularity

## Goal

Give Official Account Balance charts the same Time and API Key controls as Spend
Reports, removing the manual Frequency control.

## Decision

The balance panel reuses the existing dashboard time presets, custom-date inputs,
API-key option list, and right-aligned `report-filters` presentation. This is the
least surprising option because operators use an identical control vocabulary in
the adjacent reporting panel.

The balance request accepts inclusive `from`, `to`, and `api_key` filters. Time
drives frequency: Today and Yesterday request hourly buckets; Last 7 Days, Last
30 Days, This Month, Last Month, and Custom request daily buckets. Monthly is no
longer exposed. API-key filtering is resolved by the server using the existing
configured non-secret API-key name-to-reference mapping.

## Data and behavior

- The frontend loads balance charts separately when a balance filter changes and
  retains existing data until the replacement request succeeds.
- Custom dates require both bounds and a non-inverted inclusive range; invalid
  values show an inline balance-filter error and make no request.
- The backend filters balance snapshots by workspace day and account API-key
  reference. It groups snapshots by the requested hour or day and computes CNY
  spending against the preceding bucket within each account and selected range.
- API-key names only are returned to the browser; secret values remain server-side.

## Verification

- Handler and store tests cover balance date/API-key filters and hourly/daily
  validation.
- Client tests cover query serialization.
- Svelte checks, lint, focused Go tests, and a production web build pass.
