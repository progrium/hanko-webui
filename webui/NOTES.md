# Schema → UI rules

How [webui/admin.html](./webui/admin.html) turns the embedded OpenAPI 3 spec into the admin UI. Each rule is marked:

- **generic**: follows from the spec alone and should carry over to other APIs.
- **heuristic**: a convention guess that happens to fit Hanko's API.
- **hanko**: hard-coded for this API.

## 1. Loading the spec

- The spec is embedded as `<script id="schema" type="application/json">` and parsed at startup. It is not fetched from the server. (hanko)
- `resolve(schema)` (generic):
  - Follows `$ref` (local `#/...` pointers only).
  - Flattens `allOf` by merging the parts: properties are merged together, the `required` lists are combined, and later parts override other keys.
  - `oneOf`/`anyOf` are not handled.
- Operations: every `get/post/put/patch/delete` under `paths`, flattened to `{...operation, path, method}`. (generic)
- Parameters: an operation's `parameters`, each resolved. Path-level parameters are ignored. (generic)

## 2. Routing model

- **Screens are GET operations.** The current screen is a path template (`S.path`, e.g. `/users/{id}/emails`) plus a params map (`S.params`, e.g. `{id: "..."}`). Rendering a screen means calling that path's GET. (generic)
- **Response shape decides the view.** An array becomes a table, and anything else becomes formatted JSON. (generic)
- **Path shape:** placeholder names are ignored when comparing paths, so `/organizations/{id}` and `/organizations/{org_id}/users` share the base `/organizations/{}`. (generic)
  - `getLike(path)` finds the GET whose path has the same shape.
  - `mapParams(from, to)` carries parameter values across by position, so `id` becomes `org_id` when moving from `/organizations/{id}` to `/organizations/{org_id}/users`.
- **Top-level sections** are a hand-written list of `[label, path]` pairs: Users, Organizations, Roles, Webhooks, Audit logs, Status (`/`), Metrics. (hanko)
  - They could be derived from the distinct first path segments.
  - The list sets the order and labels.
  - A section's operations are those whose path equals the section path or starts with it plus `/`.

## 3. Labels

- `human(name)` turns field, parameter and path-segment names into labels: (generic)
  - Splits snake_case, kebab-case and camelCase into words.
  - Capitalizes only the first word ("Created at").
  - Uppercases known acronyms: `id`, `url`, `uri`, `api`, `ip`, `json`, `jwt`, `mfa`, `otp`, `totp`, `saml`, `oidc`, `sso`, `http(s)`, `ui`, `uuid`, `aaguid`. `ids` becomes "IDs".
- A small override table for names that convert badly: (hanko)
  - `q` → Search
  - `webauthn_credentials` → WebAuthn credentials
  - `role_id_or_slug` → Role ID or slug
  - `meta_*` → drops the `meta_` prefix
  - `is_primary` / `is_verified` → Primary / Verified
- Only labels are humanized. Raw names stay in JSON views, request bodies and the path subtitle. (generic)

## 4. Page header

- **Breadcrumbs** come from the current path's segments, excluding the last one: (generic)
  - Literal segments use `human()`.
  - `{param}` segments show the shortened param value: first 8 characters plus "…", with the full value on hover.
  - A crumb is a link only when a GET exists for that prefix.
- **Title:** (heuristic)
  - If the path ends in a literal segment, the title is that segment (or the section label when it's a section root).
  - If it ends in `{param}`, the title is the record's display label.
    - Display label: first non-empty of `name`, `username`, the primary email's `address`, the first email's `address`, `address`, `slug`, `callback`.
    - A tag shows the singular of the parent segment ("Users" → "User").
    - Without a display label, the singular is used as the title.
- **Subtitle:** the raw path template in monospace, directly under the title. (generic)
- **Icon actions** (round buttons, right-aligned): (generic)
  - **Filter**: only when the screen's GET has query parameters. It has a badge counting the active non-`page` filters.
  - **Refresh**: re-runs the GET and spins while loading.
  - **⋯ All operations**: every operation in the section, with colored method tags. A menu entry opens that operation's form with no parameters filled in.
- **Header buttons**: non-GET operations on the current path, plus POSTs to `current/{x}` that have no GET of their own. The second kind is "action" endpoints such as `POST /organizations/{org_id}/users/{user_id}` → "Add organization member". (generic)
  - Order: POST, then PATCH/PUT, then DELETE.
  - POST is the blue primary button with a ＋ icon, and DELETE uses the danger style.
  - Labels are shortened from the operation `summary`: drop the articles "a/an/the" and the word "new", and drop trailing "by ID" / "for/of/from/to (a) user". "Create a new user" becomes "Create user". (heuristic)
- **Sub-resource tabs**: shown when the current path is a record (`…/{x}`) or a direct child of one. (generic)
  - The tabs are "Details" (the record's own GET) plus every GET at `record/<literal>` with no further segments.
  - Each tab is labelled with `human()` of its last segment.
  - The active tab is the one whose path has the same shape as the current path.

## 5. Lists (tables)

- **Columns:** (heuristic)
  - Columns are the union of keys across the rows, priority keys first, capped at 6.
  - Priority keys: `id`, `address`, `name`, `slug`, `username`, `emails`, `type`, `actor_email`, `callback`, `enabled`, `user_id`, `created_at`.
  - Headers use `human()`.
- **Cells:** `null` shows as "—", objects and arrays as compact JSON, and everything else as a string. (generic)
- **Row actions:**
  - **Open** goes to the detail GET `path/{x}`. The row's ID is `row.id || row.user_id || row.role_id`. (heuristic)
  - **JSON** shows the raw row. (generic)
  - If there is no detail GET, the row instead lists every operation under `path/{x}…` up to two segments deep as buttons labelled with their summaries. GETs navigate and the others open forms. (generic)
- **Pagination:** appears when the GET has a `page` query parameter. (heuristic)
  - The total comes from the `X-Total-Count` response header.
  - **Next** is disabled when `page × per_page ≥ total`. Without a total, it's disabled when the page returned fewer than `per_page` rows (default 20).
  - Changing any other filter resets `page` to 1.
- **Special case:** `/webhooks` returns `{database, config}`, so it renders as two tables, and the config table is read-only. (hanko)

## 6. Filters

- The filter fields are the screen GET's query parameters, shown as compact text inputs in a dropdown panel. The placeholder is the parameter's `schema.default`. (generic)
- Array parameters are sent as repeated keys, from comma-separated input. Empty values are omitted. (generic)
- "Apply filters" closes the panel and reloads. Clicking outside closes it. (generic)

## 7. Operation forms (popups)

### Structure

- **Header:**
  - The title is `summary`, with `METHOD /path` as a monospace subtitle. (generic)
  - On the right are round icon buttons: `{ }` toggles raw JSON (only shown when the operation has a request body) and `×` closes.
  - The body scrolls inside the rounded dialog, and the header stays fixed.
- **Description:** the operation `description`, as muted text. (generic)
- **Path parameters** come first, prefilled from the current screen's params. Required ones are marked `*`. (generic)
- **Body fields** come from the resolved `requestBody.content['application/json'].schema.properties`. (generic)
- **Submit label:** the first word of `summary` (Create, Update, Delete, Remove, Set, Add, Patch…) ("Open" for GET, "Execute" as a fallback). (heuristic)
- DELETE asks for confirmation. If you delete the record you're viewing, you go back to the section root. Otherwise the current screen reloads. (generic)
- A response containing `session_token` is shown in a result popup instead of being discarded. (hanko)

### Form mode, by HTTP method (generic)

| Method | Mode | Initial values | What's sent |
|---|---|---|---|
| POST | create | schema `default`, otherwise empty | non-empty fields only; required fields must be filled |
| PATCH | update | the record's current values | only fields that differ from the original; clearing a field sends `null` |
| PUT | replace | the record's current values | every field, including empty ones |

- **Update forms:** (generic)
  - A changed field gets a blue left bar and a **Reset** link.
  - The footer shows "N changes", and submit is disabled until something has changed.
  - Fields are compared after normalizing: empty string and `null` count as equal, and JSON is compared parsed.
- **Labels:** required fields get a red `*`. Optional fields on create forms get a grey "optional". Update forms show neither. (generic)
- **Hints:** the schema `description` under each field, minus the boilerplate "Send `null` to remove…". The × button covers that. (heuristic)
- **Raw mode:** switching to raw fills the editor with the current form values as JSON. The JSON is then sent unchanged. (generic)

### Field controls, by schema type (`kind()`, generic)

| Schema | Control | Notes |
|---|---|---|
| `type: string` with `enum` | select | first option "— Select —" (or "Not set" when nullable) |
| `type: string` | text input | `format` sets the input type: `email` → email, `uri` → url; a name containing `password` → password |
| `integer` / `number` | number input | checked on blur; integers must be whole numbers |
| `boolean`, required or on edit forms | switch | shows "Yes"/"No" |
| `boolean`, optional on create | Default / Yes / No buttons | so "don't send" can be chosen |
| `array` of `enum` strings | checkbox grid | "N of M selected" and Select all / Clear all |
| `array` of strings | tag chips | Enter or comma adds; Backspace removes the last chip; duplicates are ignored |
| `array` of objects | one card per item | each item's properties are rendered recursively; × removes; "+ Add {singular}" appends |
| `object` / has `properties` | collapsed JSON editor | one-line preview; opens into a textarea |

- **Type detection order:** `type: string` is checked first. Because `allOf` merging can leave an empty `properties: {}` behind, an object needs either `type: object` or at least one property. (generic)
- **Fields that can be null** (on update and replace forms) get a small × inside the input. Clearing shows an italic "Not set" placeholder and sends `null`. (generic)
- **Placeholders:** the schema `example` if it's a scalar. Otherwise based on `format`: email, uri, date-time, uuid/uuid4. (generic)
- **Validation** runs on blur and on submit. (generic)
  - Errors appear under the field, in red with a red bar.
  - Checks: valid email, parseable URL, valid number, valid JSON, required fields, and required properties inside array items.
- **New array items:** a new item starts from its properties' blank values. `is_primary` defaults to true on the first item only. (hanko)

## 8. Requests (generic)

- All requests go to `/admin/api` + the path with parameters filled in (URL-encoded) + the query string.
- A missing path parameter is an error.
- Headers:
  - `X-Admin-UI: 1` on every request, which the Go proxy requires for CSRF protection.
  - `Content-Type: application/json` only when there's a body.
- A 401 response sends the browser to `/signin?next=<current page>`.
- Error message: the response's `message` field, otherwise the response text, otherwise the status text. Redirects are treated as errors.
- Overlapping requests: only the latest load's response is applied (a sequence number is kept).

## 9. Visual conventions

- **Top header:** a dark band with the app name. Below it are square section tabs; the active tab takes the page background so it joins the content.
- **Buttons:** icon buttons are 32px circles, and the active state is solid blue.
- **Method colors:** GET green, POST blue, PATCH/PUT amber, DELETE red.

## Possible generalizations

- Derive the top sections from the spec's first path segments or `tags`, instead of the hand-written list.
- Derive the row ID from the detail path's parameter name, not `id || user_id || role_id`.
- Choose display-label fields and priority columns from schema hints, e.g. an `x-ui-label` / `x-ui-columns` vendor extension, or the first required string property.
- Support `oneOf`/`anyOf` (e.g. a type switcher) and `additionalProperties` maps (a key/value editor instead of JSON).
- Format `date-time` cells and show arrays of objects (like `emails`) as chips in tables.
