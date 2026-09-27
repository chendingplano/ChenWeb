# Current User Profile in the Home3 Navigation Rail

## Goal

Show the authenticated user's actual name and email in the lower-left profile area of the `/development` and `/home3` navigation rail, and in the page opened by the navigation rail's User Info menu item.

## Design

The existing `GET /api/v1/ai-assistant/user-info` route is already protected by the API group's authentication middleware, but its handler currently returns a sample profile. Change the handler to read `ApiTypes.UserInfo` from `EchoFactory.NewFromEcho(c, ...).IsAuthenticated()` and return an explicit, minimal `{ "user": { "name": ..., "email": ... } }` projection. Do not serialize the full auth object, which contains sensitive fields. Return an authentication error if the context has no authenticated user.

The Home3 `NavRail` will fetch this endpoint when mounted and store the returned name and email in component state. It will show the joined first and last name when present, otherwise the account name, then the email. In the current auth mapping, the account name may itself be the email address, so the real profile name takes priority. The initials use the displayed name, falling back to email when needed. During loading or if the request fails, the profile area stays empty rather than showing another user's identity.

The User Info menu action already selects the `__user_info__` item. `ContentPanel` will route that item to a dedicated view that fetches the same endpoint and displays the current user's name and email, with loading, error, and retry states. This replaces the generic placeholder rendered for unknown sections.

## Scope and verification

Changes are limited to the existing user-info handler, Home3 nav rail, and the dedicated User Info view and routing branch. No database or auth schema changes are needed. Verify the frontend type check and Go package build; inspect the response contract and UI loading/failure behavior.

## Knowledge and documentation

The API contract now represents the authenticated user rather than sample data. No other docs or ADRs describe this endpoint's placeholder identity, so no additional documentation changes are expected. Intentionally undocumented: profile editing and avatar loading, which are outside this request.
