# Holiday Display Sequence Design

## Goal

Allow administrators to control the order of holiday definitions independently for each
country using a unique, positive `display_seqno`.

## Design

- Add `display_seqno INT NOT NULL` to `public.holiday_info`.
- Backfill existing rows per country with contiguous values starting at 1, ordered by the
  existing record ID, then enforce `UNIQUE (country, display_seqno)`.
- Creating a holiday assigns one more than the current country maximum inside a transaction.
- Updating a holiday accepts a positive sequence number. The affected country’s rows are
  shifted transactionally so the edited holiday occupies the requested position and all
  sequence numbers remain unique and contiguous.
- Moving a holiday between countries removes it from its old country’s sequence and inserts it
  at the requested position in the new country. If no valid position is supplied by the client,
  the API uses the new country’s next available position.
- Listing holiday definitions orders by `display_seqno` and uses `id` only as a defensive tie
  breaker.
- The admin table displays the sequence number and the edit form allows it to be changed.

## Error handling

- `display_seqno` must be a positive integer.
- Invalid sequence values return HTTP 400.
- Database uniqueness conflicts return the existing holiday-info conflict envelope.
- Sequence updates use one transaction so a failed update leaves the prior ordering intact.

## Verification

- Add Go store tests for append numbering, ordered listing, moving within a country, and
  moving between countries.
- Add migration/schema checks where the project’s existing test conventions support them.
- Run the calendar handler Go tests, `go build`, and the frontend `svelte-check` command.

