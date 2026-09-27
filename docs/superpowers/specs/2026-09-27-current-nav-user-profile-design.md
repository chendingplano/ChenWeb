# Current User Profile in the Home3 Navigation Rail

## Goal

Show the authenticated user's details in the page opened by the navigation rail's User Info menu item: first name, last name, email, phone number, roles, create time, and last login time. The lower-left profile area continues to show name and email.

## Design

The existing `GET /api/v1/ai-assistant/user-info` route is already protected by the API group's authentication middleware and supplies name/email to the navigation rail. The User Info page reads `/auth/me`, which already returns the current authenticated session, identity traits, roles, account creation time, and session authenticated-at time. Add the missing phone trait to the explicit Kratos session response. This avoids serializing the full auth object, which contains sensitive fields.

The Home3 `NavRail` will fetch this endpoint when mounted and store the returned name and email in component state. It will show the joined first and last name when present, otherwise the account name, then the email. In the current auth mapping, the account name may itself be the email address, so the real profile name takes priority. The initials use the displayed name, falling back to email when needed. During loading or if the request fails, the profile area stays empty rather than showing another user's identity.

The User Info menu action already selects the `__user_info__` item. `ContentPanel` routes that item to a dedicated view that fetches the current session and displays the requested seven fields, with loading, error, and retry states. Timestamps are formatted in the browser's local timezone. Missing optional values display as an em dash.

## Scope and verification

Changes are limited to the shared Kratos session response and ChenWeb's dedicated User Info view and routing branch. No database or auth schema changes are needed. Verify the frontend type check and Go package builds; inspect the response contract and UI loading/failure behavior.

## Knowledge and documentation

The API contract now represents the authenticated user rather than sample data. No other docs or ADRs describe this endpoint's placeholder identity, so no additional documentation changes are expected. Intentionally undocumented: profile editing and avatar loading, which are outside this request.
