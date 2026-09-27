# Current User Profile in the Home3 Navigation Rail

## Goal

Show the authenticated user's actual name and email in the lower-left profile area of the `/development` and `/home3` navigation rail.

## Design

The existing `GET /api/v1/ai-assistant/user-info` route is already protected by the API group's authentication middleware, but its handler currently returns a sample profile. Change the handler to read `ApiTypes.UserInfo` from `EchoFactory.NewFromEcho(c, ...).IsAuthenticated()` and return an explicit, minimal `{ "user": { "name": ..., "email": ... } }` projection. Do not serialize the full auth object, which contains sensitive fields. Return an authentication error if the context has no authenticated user.

The Home3 `NavRail` will fetch this endpoint when mounted and store the returned name and email in component state. It will show the account name when present, otherwise the joined first and last name, then the email. The email is shown as provided by the authenticated profile. The initials use the displayed name, falling back to email when needed. During loading or if the request fails, the profile area stays empty rather than showing another user's identity.

## Scope and verification

Changes are limited to the existing user-info handler and the Home3 nav rail. No database or auth schema changes are needed. Verify the frontend build/type check and relevant Go package tests; inspect the response contract and UI loading/failure behavior.

## Knowledge and documentation

The API contract now represents the authenticated user rather than sample data. No other docs or ADRs describe this endpoint's placeholder identity, so no additional documentation changes are expected. Intentionally undocumented: profile editing and avatar loading, which are outside this request.
