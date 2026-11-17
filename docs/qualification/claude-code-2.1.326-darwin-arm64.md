# Claude Code 2.1.326 darwin-arm64 artifact check (gateway admission contract A)

Read-only artifact analysis, 2026-11-17, revised after review of the first
draft of this report and again after the review of record for this change.
Published npm `@anthropic-ai/claude-code-darwin-arm64`
tarballs at 2.1.316, 2.1.318 and 2.1.326 were unpacked as data and searched
only; nothing was executed. Counts are literal occurrences in the extracted
binary (`grep -a -o -F`, one token per pass unless noted), calibrated
against the prior 2.1.319 report's counts for 2.1.316 and 2.1.318, which
reproduce exactly. Context was read from `strings` output. Minified
identifiers differ between builds and are reused in unrelated chunks of one
build; where a snippet is quoted, the build is named, and an identification
that rests on a name shared across chunks is marked as a fingerprint, not a
proof of identity.

**Verdict: 2.1.326 is grounded together with the launcher change that
carries this record.** Grounding is not effective until that change lands,
an operator records the 2.1.326 binary hash below in the strict tuple
with this record as its evidence, and the README's runtime
no-direct-connection witness has been run for this build; a hash entry
alone does not establish these semantics, and the hash must not be
recorded without the launcher change. This evidence supports an `exec`
(print) strict-tuple entry. It does not by itself qualify an
`interactive` entry: the interactive session mounts UI setup effects this
check mostly does not exercise, one of which (the official-marketplace
auto-install fetch, claim 6) is named there, so an `interactive` entry
needs its own recipe evaluation and witness even when it names this
record. Three findings of the first draft change verdict on tracing: the
connector download path is statically unreachable inside the gateway
boundary (claim 2d, with the runtime no-direct-connection witness kept as a
written residual), the eager scanner's value-slot coverage of the allowlist
is complete and the initialize `managedSettings` field is ignored without
its flag (claim 4), and the legacy global-file choosers agree in every
exact-name state a contract-A home can hold (claim 5b). Every remaining
DIFFERS is either a build-bounded README sentence or is closed fail-closed
by the launcher change described under "What this grounding required".

## Artifacts

| Build | Tarball sha256 | Tarball bytes | Binary sha256 | Binary bytes |
| --- | --- | ---: | --- | ---: |
| 2.1.316 | `3515788bc9db6c52ef8edb776228ce647de1af710efab85d289782a872147baa` | 141,330,970 | `64c327e92b2c5e0f78c0a980c5a518121e1f0d140ca771ca5dd4488d1a2dcb3f` | 310,114,176 |
| 2.1.318 | `02a686b84bbd96a6fd33263770e784aa5a959c6287e669e298667f23e75a96ba` | 145,803,514 | `677a2ea7bf576cedd20e191d973914ed8a41796f53a63b1923381ddc4d5b5e7d` | 319,598,464 |
| 2.1.326 | `bda994b80919b5df9588b81c98b8f8a08aa4ca6ba605efda74679ed8571e8136` | 160,222,918 | `e214ad76dcc0e846b03be28ed1c2734aacd4e27acb527cb7f8a9e2bed644e837` | 353,037,664 |

Each tarball contains `package/claude`, `package/package.json`,
`package/LICENSE.md`, `package/README.md`; each `claude` is a Mach-O 64-bit
arm64 executable. The sha256 and sha1 above were recomputed here; the sha1
values match the registry `dist.shasum` in the staged registry metadata
(2.1.316 `c8f641b1300255f83baaa033491658e297a2cbb1`, 2.1.318
`f420e68f24e5cd40f20fa00dd1a02ccf13a1e86c`, 2.1.326
`58c6d4a6fdedb718d67a7bd7537d759d69340dda`), and the sha256 and sizes match
the staged checksum record, which also records the registry sha512
integrity as verified. The record proves provenance only; it says nothing
about runtime behaviour, and no finding here is a runtime or connection
witness.

## Claim table

| # | Claim (2.1.326 against 2.1.316/2.1.318) | Result |
| --- | --- | --- |
| 1a | Managed-preferences plist identifier and per-user/device paths | HOLDS |
| 1b | Policy-cache file names and the six companions | HOLDS |
| 1c | Types of the launcher-admitted closed subset, incl. `monitoring_notice` catching a string to `null` | HOLDS |
| 1d | Native policy object schema member list and per-member handling | DIFFERS: new member `upcoming_minimum_version` (differs from the check, not from a README sentence) |
| 2a | `enforce_web_search_mcp_isolation` consumer shape | HOLDS |
| 2b | `allow_connector_file_downloads` has no literal consumer; generic evaluator only | DIFFERS: policy-table entry plus 3 literal evaluator calls. The README sentence is build-bounded; the admitted value stays inert under gateway ineligibility |
| 2c | Nothing new in an admitted cache selects a route, settings source or permission mode | HOLDS (the native restriction universe grew; every new key is refused by the closed subset) |
| 2d | Connector downloads are unreachable inside the gateway boundary | HOLDS on the traced paths (static proof in claim 2; runtime witness retained as a residual) |
| 3 | First-party base-URL gating of the principal policy read/fetch/poll paths, first-party assumption writer, custom-base early return, hinted-enforcement gate | HOLDS (static observation, as the README phrases it; one new same-endpoint feedback fetch is recorded there) |
| 4a | Every `gatewayNativeOptions` entry still declared with the same value shape | HOLDS |
| 4b | Options declared in 2.1.326, not allowlisted, new since 2.1.314 | HOLDS: the allowlist refuses them as designed; inventory in claim 4. Dating against 2.1.314 itself CANNOT TELL (no 2.1.314 artifact staged; 2.1.316 is the earliest staged proxy) |
| 4c | Reserved/dispatch words the runtime searches for outside its option parser | DIFFERS: new raw token and new command names; covered by the reservations in the launcher change |
| 5a | `settings.json` / `cowork_settings.json` selection and `.claude.json` path under `CLAUDE_CONFIG_DIR` | HOLDS |
| 5b | Legacy `.config.json` handling | DIFFERS: the chooser mechanism expanded. Exact-name contract-A states choose the same file as 2.1.316; the one build-divergent state is refused by the launcher change |
| 5c | Exact predicate of the new project/local settings-path suppression guard | CANNOT TELL in leaf detail; its only effect is to return no path, and the launcher already checks a superset of those files |
| 6a | The signed model-catalog fetch, its gate and its `CLAUDE_CODE_MODEL_CATALOG=off` switch are identical in all three builds, and the fetch is off under the gateway's composed environment | HOLDS (static; every `downloads.claude.ai` occurrence in 2.1.318/2.1.326 is classified in claim 6, and each gate predicate is identified there) |
| 6b | The observed direct `downloads.claude.ai` connection is that catalog fetch | CANNOT TELL, and not the catalog fetch on this evidence: in the observed launch's environment the catalog gate already returned `essential_traffic`. One other startup fetch to the host is not gated by the four gateway conditions (official-marketplace auto-install); which path fired is for the runtime witness to decide |

## Literal occurrences in the bundled binary

| Token | 2.1.316 | 2.1.318 | 2.1.326 |
| --- | ---: | ---: | ---: |
| `com.anthropic.claudecode` | 2 | 2 | 2 |
| `/Library/Managed Preferences` | 3 | 3 | 3 |
| `remote-settings.json` | 8 | 8 | 8 |
| `policy-limits.json` | 11 | 11 | 11 |
| `.signature.json` | 2 | 2 | 2 |
| `.signature-iat.json` | 2 | 2 | 2 |
| `.stamp.json` | 2 | 2 | 2 |
| `.synced-at` | 2 | 2 | 2 |
| `allow_connector_file_downloads` | 0 | 0 | 5 |
| `enforce_web_search_mcp_isolation` | 2 | 2 | 2 |
| `allow_cobalt_plinth_alkanet` | 0 | 0 | 2 |
| `Whiteboard` | 0 | 0 | 10 |
| `remote_control_at_startup` | 2 | 2 | 2 |
| `compliance_taints` | 41 | 41 | 42 |
| `monitoring_notice` | 6 | 6 | 6 |
| `core_skills` | 9 | 9 | 9 |
| `artifact_create_notice` | 5 | 5 | 5 |
| `upcoming_minimum_version` | 0 | 0 | 3 |
| `feedbackRefusal` | 0 | 0 | 5 |
| `CLAUDE_CODE_ASSUME_FIRST_PARTY_BASE_URL` | 17 | 17 | 17 |
| `assumesFirstPartyBaseUrl` | 10 | 10 | 10 |
| `assumesFirstPartyBaseUrl=!0` | 1 | 1 | 1 |
| `assumesFirstPartyBaseUrl=!1` | 1 | 1 | 1 |
| `assume_first_party_base_url` | 8 | 8 | 8 |
| `isFirstPartyAnthropicBaseUrl` | 2 | 2 | 2 |
| `custom_base_url` | 27 | 29 | 29 |
| `--client-data-url` | 17 | 19 | 19 |
| `CLAUDE_CODE_CLIENT_DATA_URL` | 14 | 17 | 17 |
| `ANTHROPIC_UNIX_SOCKET` | 66 | 73 | 76 |
| `settings.json` | 453 | 467 | 502 |
| `cowork_settings.json` | 4 | 4 | 7 |
| `.config.json` | 9 | 9 | 11 |
| `.claude.json` | 78 | 88 | 88 |
| `CLAUDE_CONFIG_DIR` | 97 | 97 | 112 |
| `getUnusedGlobalClaudeFile` | 0 | 0 | 3 |
| `getProbedLegacyGlobalConfigFile` | 0 | 0 | 3 |
| `--ignore-persisted-trust` | 0 | 0 | 6 |
| `CLAUDE_CODE_IGNORE_PERSISTED_TRUST` | 10 | 10 | 11 |
| `worktree-hook` | 0 | 0 | 2 |
| `workspace-trust` | 0 | 0 | 4 |
| `restore-set-aside` | 0 | 0 | 7 |
| `drop-worktree-registrations` | 8 | 8 | 0 |
| `--managed-settings-in-initialize` | 0 | 0 | 22 |
| `connector-downloads` | 0 | 0 | 113 |
| `prosumer_oauth` | 7 | 7 | 10 |
| `policy_not_asked` | 5 | 5 | 8 |
| `CLAUDE_CODE_MODEL_CATALOG` | 13 | 13 | 12 |
| `CLAUDE_CODE_MODEL_CATALOG_URL` | 4 | 4 | 4 |
| `downloads.claude.ai` | 30 | 30 | 29 |
| `https://downloads.claude.ai/model-catalog/v1/catalog.json` | 3 | 3 | 3 |
| `allow_model_catalog` | 6 | 6 | 6 |
| `tengu_delegated_quail` | 2 | 2 | 2 |

Full companion literals such as `remote-settings.json.signature.json` occur
0 times in all three builds because the names are constructed from suffixes;
the suffix builders are quoted under claim 1. Load-bearing counts are
paired with quoted code below. For two high-churn tokens
(`ANTHROPIC_UNIX_SOCKET`, `CLAUDE_CONFIG_DIR`) the delta is accounted by
rename-invariant signature counts and role classification under claims 3
and 5, and no verdict rests on those counts alone; for `settings.json`
and `downloads.claude.ai` each net-new or raw occurrence in
2.1.318/2.1.326 is individually attributed under claims 5 and 6.

## Claim 1: plist, cache files, closed subset, native schema

The plist identifier is a single constant in all three builds
(`t="com.anthropic.claudecode"`), expanded into the same two paths:

- per-user: `` `/Library/Managed Preferences/${username}/${t}.plist` ``
  (label `per-user managed preferences`)
- device-level: `` `/Library/Managed Preferences/${t}.plist` ``
  (label `device-level managed preferences`)

The full literal with `.plist` suffix occurs 0 times in every build because
it is templated.

Cache and companion construction is token-identical modulo renames. In
2.1.316 / 2.1.326 respectively: `` `${e}.signature.json` ``,
`` `${e}.signature-iat.json` ``, applied to both cache names
(`["remote-settings.json","policy-limits.json"].flatMap(...)`); `pat`/`tIt`
returns `` `${e}${".stamp.json"}` ``, called as `pat("policy-limits.json")`;
`f3e`/`cct` returns `e+".synced-at"`, called as `f3e("remote-settings.json")`.

The native policy-limits object schema (a single object-schema definition
in each build) is:

| Member | Native schema shape (all builds unless noted) | In launcher subset |
| --- | --- | --- |
| `restrictions` | record of string to `{allowed: boolean}` | Only the two entries in claim 2 |
| `compliance_taints` | array, default `[]`, transformed to the normalized string entries (capped); entries with none well-formed add a schema issue | Empty array only |
| `monitoring_notice` | `{text: string≤500 (sanitized), url: https URL≤2048, nullish, per-field catch to null}`, then `.nullable().default(null).catch(null)` | `null` or string; a string fails the object schema and is caught to `null` in 2.1.326 exactly as in 2.1.316 |
| `defaults` | record of string to an unconstrained value, `.default({}).catch({})` | Only `remote_control_at_startup: false` |
| `core_skills` | array of string≤128, ≤256 entries, optional, catch `[]` | No (refuses) |
| `artifact_create_notice` | `{text: string (truncated, sanitized, non-empty)}`, nullish, catch to a fixed approval sentence | No (refuses) |
| `upcoming_minimum_version` | 2.1.326 only: `{claude_code: version regex, date: date regex}`, nullish, catch `null` | No (refuses) |

The schema definition and the empty default document
(`{restrictions:{},compliance_taints:[],monitoring_notice:null,defaults:{}}`)
were read directly in 2.1.326. `upcoming_minimum_version` consumers found
there are display-only: a getter returns the session-cache value unless the
verdict is disowned; a builder renders a sentence of the form "Claude Code
*X* or later is required for your organization" (with an "upcoming" state
before the date), shown as a start-up warning, a status warning (id
`minimum-version`), and a "Minimum version" status row. No extracted
consumer selects a route, a settings source or a permission mode. The
launcher's closed subset refuses a cache that carries the member (unknown
member), so an admitted launch is unaffected, but a real 2.1.326 cache that
carries it refuses every gateway launch: fail-closed, at availability cost.
The decision taken with this grounding is to keep refusing; admitting the
display-only member would need its own proof and test vectors.

The `compliance_taints` delta (41/41/42) is one new property read. All
other signature counts are unchanged (quoted 3/3, key position 11/11, bare
string-table entry 1/1; `lossy_compliance_taints` 2/2, bodyTaints 4/4). The
new read is in a new feedback-access check: the fetched policy document is
parsed and `n.data.compliance_taints.includes("hipaa")` gates latching a
feedback refusal when `allow_product_feedback` is denied (the
`feedbackRefusal` machinery is new in 2.1.326, 0/0/5). An admitted empty
array reads `false` exactly as in the grounded builds, so claim 1c holds;
the fetch itself is recorded under claim 3.

The only key read through the native defaults getter is
`remote_control_at_startup` in all three builds, with the same consumer: an
org-policy `false` yields `{value:false, source:"org_policy"}` ahead of the
feature-flag fallback. `remote-settings.json` admitted as exactly `{}` is a
launcher-side restriction; nothing in 2.1.326 changes the file name, its
eligibility function (claim 3), or the selectors
`CLAUDE_CODE_REMOTE_SETTINGS_PATH` (8/8/8) and
`CLAUDE_CODE_MANAGED_SETTINGS_PATH` (9/9/9), both in the closed environment
namespace the launcher refuses and removes.

## Claim 2: the two admitted booleans, the restriction universe, and the connector download paths

### 2a. Isolation

`enforce_web_search_mcp_isolation` (2/2/2) is the same consumer family in
all three builds: the key constant (`pd`), the same feature-flag gate
(`tengu_doorbell_agave`), and the enforced-or-hinted helper `i8t`, whose
hinted branch still ends `&& xy()` (`isPolicyLimitsEligible`). In 2.1.326
its readers, quoted from the bundle, are:

- `fa() = !md() && i8t(pd)` — the key's own shape, unchanged;
- `SS() = !md() && (i8t(pd) || _w())`, where `_w()` (policy verdict absent)
  itself returns `!1` when not `xy()`;
- the new compound `iXn() = fa() || lastingConnectorDownloads()==="held" && SS()`;
- `aXn(...)`, which returns null when `md() || !xy() || B4() && !i8t(pd)`.

There are exactly three `i8t(pd)` occurrences in 2.1.326 and no literal-key
`En`/`ssl` consumer. Every one is `false` or null-returning when policy is
ineligible: the gated cache read is null, so enforced misses, hinted is
cut by `&& xy()`, and `_w()` is false. Isolation `false` ≡ absent
therefore holds for every 2.1.326 consumer, including the compound.

### 2b. Connector key consumers; the cache value stays inert

The README sentence "The published artifact has no literal consumer for
the connector key" holds for 2.1.316 and 2.1.318 (0 occurrences) and is
false for 2.1.326 (5 occurrences). All five contexts were read here:

- a policy-table entry: `deniedUnder:["hipaa","zdr"]`,
  `onCacheMiss:"deny"`, `onFailOpenVerdict:"deny"`, label "Downloading
  files from claude.ai connectors", `HIPAA-R114`;
- three literal `En("allow_connector_file_downloads")` calls (`En` is
  `isPolicyAllowed`): in the direct-MCP classification
  `cdn(e)`, in the download-offer refusal chain, and in the shared
  refusal-reason helper `W0s()`;
- one bare string-table entry among the other policy key and field names,
  with no consumer context (line 676320 of the `strings` extract, flanked
  by `allow_memory_sync`, `onFailOpenVerdict`, `allow_projects_tool`).

The evaluator is quoted in 2.1.326: `Ce(e)` reads `n=xy()`, `i=yT(n)`; with
no cache read, a fail-closed-on-miss key returns `!1` only when
`(n||xy())` (eligible) or for the single hardcoded feedback key; with no
hinted taints or denials it then returns `!0`, and `if(!(n||xy()))return!0`
covers the ineligible case explicitly. So in an admitted gateway session,
where claim 3 keeps eligibility off, `isPolicyAllowed` for this key is
`true` whether the cache says `true`, `false` or nothing. The value admits
and denies nothing by itself; narrowing the admitted cache value would
close nothing. The generic response-path evaluator (`lIt`: taint check,
then `restrictions[key].allowed`, then taint-table fallback) is unchanged
but is no longer the key's only consumer. What the tracing changes is
1) the README's no-literal-consumer sentence, now build-bounded to
2.1.316/2.1.318, with the eligibility condition stated for all three
builds, and 2) the download reachability question, answered next. This
consumer has been known since 2.1.319: the earlier 2.1.319 report (not
reproduced here; no 2.1.319 artifact was staged for this revision) recorded
4 occurrences and proposed a cache narrowing that was not adopted.

### 2d. Reachability and route, both connector paths

2.1.326 adds connector file downloads (`connector-downloads` 0/0/113;
`via:"direct"` 0/3, `via:"claude-ai"` 0/1 tokens in the same order). There
are two paths to judge: the download-offer path (an MCP tool result offers
files, a person may approve, bytes are fetched and saved) and the
direct-MCP gate (`cdn(e)` returns `{via:"direct", mcpUrl}`).

Offer path. Offers are built only when downloads are on for the server
(`downloadsOn:aMr(config)`; `aMr = bSe() && cdn(config)!==void 0`, a feature
gate whose static default the first draft read as off). Each offered file
then passes one refusal value computed per result, quoted in order:

`!ez() ? "not_allowed_here" : no session token or no claude.ai sign-in
? "no_session_token"/"no_sign_in" : U ? "policy_not_asked" :
!En("allow_connector_file_downloads") ? "policy_not_loaded"/"org_policy" :
Aie() ? "isolation_policy" : !pe() ? "training_mark_off" : void 0`

A file is fetched only when that value is `void 0` (and passes the later
per-file link, folder and person checks, which are downstream and never
reached here). `U` is the chunk-local helper
`_e(){ e=LL(); return e!==void 0 && e!=="prosumer_oauth" }` (a throw is
caught to the string `"unknown"`, still not `prosumer_oauth`). `LL()` is
identified by its `prosumer_oauth` outcome, which no other function
produces: it is the eligibility function of claim 3, returning `void 0`
when eligible and otherwise a reason string. Inside the gateway boundary
`LL()` returns `custom_base_url` (or `third_party_provider` before it):
the registration-owned base is loopback, no socket or first-party
exception applies, and that branch returns before any credential branch
that could end at `prosumer_oauth`. The refusal prose for this reason says
so in words: "the organization's policy can't be checked in this session
(it is never asked for, for example with a custom API address), so
downloads are off". The shared helper `W0s()` orders the same check first.
Sign-in state, the feature gate, isolation, the training mark and cache
content therefore do not matter under the boundary: `policy_not_asked`
fires first and no offer is fetched, whatever `En` returns.

Direct-MCP gate. `cdn(e)` classifies an already-configured MCP server
entry: a claude.ai connector (`via:"claude-ai"` with its connector id) or,
via `l2r(e)`, a local- or user-scope HTTP server whose URL is https (or
loopback http), carries no userinfo and is not an Anthropic address,
gated additionally on a feature flag (`tengu_rustling_juggling_acorn`), a
claude.ai sign-in with the inference scope (`y$()`), and `En(...)`. The
returned `mcpUrl` is that server's own configured URL (plus redirect
bookkeeping for its normal MCP transport). Traced consumers of the
classification only set `downloadsOn` and build offers, which then pass
the same refusal value above; the classification opens no connection of
its own. Tool traffic to such a server is the operator-configured MCP
egress the grounded builds already have via `--mcp-config`; it is not a
new Anthropic route.

Route if the code were ever reached. The fetch implementation has a
direct arm to the connector-offered address (address-vetted; refuses
private, local, Anthropic-own and proxied addresses, because behind a
proxy the address cannot be checked) and a relay arm where bytes come only
from Anthropic's download service (`not_the_relay_address`: "the only
place Claude Code downloads from"). Either arm is a direct connection the
gateway would not filter; reachability, not the fetch address checks, is
what keeps it off. The README therefore names connector downloads beside
its existing written residual: actual gateway qualification must check
that the launched process makes no direct Anthropic connection, connector
downloads included. No cache narrowing is taken as defence in depth: under
ineligibility it changes no result (offers stop before the policy check,
and the policy check returns `true` whatever the cache holds), while
refusing `allowed=true` globally would refuse real caches under the
already-grounded builds for no effect. If eligibility itself regressed,
that would be the eligible-session behaviour the README already documents,
not a 2.1.326 widening.

### 2c. Restriction universe

Policy-table keys (`key:{deniedUnder|…`) number 37 in 2.1.316, 39 in
2.1.318, 43 in 2.1.326; no 2.1.318 table key is gone in 2.1.326. New
native restriction keys (each counted here; all refused by the launcher's
closed subset, so none can act in an admitted cache):

| Key | 2.1.316 | 2.1.318 | 2.1.326 | Note |
| --- | ---: | ---: | ---: | --- |
| `allow_connector_file_downloads` | 0 | 0 | table + consumers | See 2b/2d |
| `allow_cobalt_plinth_alkanet` | 0 | 0 | 2 | Whiteboard artifact `policyKey`, gated by `En(key)` on the whiteboard artifact type. The earlier inert-by-name reasoning for this key does not extend to this build |
| `allow_chrome_tab_group_title` | 0 | 0 | 3 | New policy-table entry vs 2.1.318 |
| `allow_data_flow` | 0 | 0 | 3 | New policy-table entry vs 2.1.318 |
| `allow_linked_computer` | 0 | 0 | 2 | New policy-table entry vs 2.1.318 |
| `allow_connector_file_uploads` | 0 | 2 | 3 | Policy-table entry arrived by 2.1.318 |
| `allow_gmail_file_references` | 0 | 3 | 3 | Policy-table entry arrived by 2.1.318 |
| `allow_tool_search` | 0 | 0 | 2 | Read through the enforced-policy verdict in a tool-search compliance check; no policy-table entry found |
| `require_connector_block_on_plugin_servers` | 0 | 0 | 5 | Read through the enforced-policy verdict for a connector-block mode (with a `…_count_only` sibling); no policy-table entry found |

Diff method: table keys extracted as `key:{deniedUnder|…` and quoted
`allow_|enforce_|require_` names diffed separately; the last two rows come
from the quoted-name diff only. Enforced-verdict readers read
`yT()?.restrictions`, which is null under ineligibility, and the subset
refuses these names in a cache either way.

## Claim 3: first-party base-URL gating

Every link in the README's proof chain is token-identical in 2.1.326 modulo
minified renames (2.1.316 name first):

| Link | Evidence (all three builds) |
| --- | --- |
| First-party assumption starts false | State initializer `clientDataUrl:{pending:void 0,document:void 0,assumesFirstPartyBaseUrl:!1}` |
| Sole enabling writer needs the blocked client-data input | Exactly one `assumesFirstPartyBaseUrl=!0` (1/1/1), in the client-data document applier: finds an entry with `clientData.assume_first_party_base_url===!0`, and refuses when already first-party, when `_CLAUDE_CODE_ASSUME_FIRST_PARTY_BASE_URL` is set, or when the host manages the provider. Exactly one `=!1` withdraw writer |
| Assumption selector | `e6()`/`wz()`: `_CLAUDE_CODE_ASSUME_FIRST_PARTY_BASE_URL ?? clientDataUrl.assumesFirstPartyBaseUrl` (env name 17/17/17) |
| Actual-first-party test | `Ou()`/`pc()`: no `ANTHROPIC_BASE_URL` means true, else the host test (`Zd`/`Ql`, exported `isFirstPartyAnthropicHost`) |
| Combined test | `Ii()`/`Ki()` (`isFirstPartyAnthropicBaseUrl`): assumed, or actually first-party |
| Credential-independent ineligibility | `Xur()`/`xnr()`: provider not `firstParty` returns `"third_party_provider"`; `!skipBaseUrlCheck && !tunnelSocket() && !Ii()/Ki()` returns `"custom_base_url"` |
| Full ineligibility order | `Uq()`/`LL()`: credential-independent reason first; then tunnel socket admits; then API-key, sign-in and subscription branches, ending in the same `prosumer_oauth` outcome. The 2.1.326 body was read in full for this revision |
| Tunnel-socket exception | `FD()`/`yB()` via `umr()`/`vZr()`: `ANTHROPIC_UNIX_SOCKET` accepted only with the same credential-presence check; the two bodies are token-identical modulo renames, including the credential argument set |
| Principal cache read | `wE()` / `yT(e=xy())`: returns null when `isPolicyLimitsEligible` is false, before any disk read; extracted callers pass the eligibility result itself |
| Principal fetch | `runFetchCycle`: `if(!Lh()/xy()) return …, {source:"ineligible",hadCache:!1}, null` before the disk-read call |
| Principal poll | `loadFor` starts polling only under `Lh()`/`xy()`; `startBackgroundPolling` returns early `if(!xy())` (quoted here) |
| Hinted enforcement | `G5t()`/`i8t()`: hinted branch ends `&& Lh()` / `&& xy()` |
| Remote-settings eligibility | Same gateway branch (`host_gateway_not_accepted` / `unpinned_gateway`, 6/6/6 each), then `third_party_provider` / `custom_base_url` via `Ii()`/`Ki()` |

Two additions are recorded against this claim:

- Feedback-access fetch. The new check of claim 1c fetches the same
  policy-limits endpoint (path literal `/api/claude_code/policy_limits`)
  through the API client and latches a feedback refusal; on the extracts it
  is not itself wrapped in `xy()`. Its route is the API base, which the
  composed environment pins to the registration-owned loopback endpoint,
  so in a gateway session the request goes to the gateway, not directly to
  Anthropic, and its only policy effect is to deny `allow_product_feedback`.
  When exactly the held-start machinery invokes it was not fully traced.
  It is not one of the principal read/fetch/poll paths the README
  describes, and it does not widen what a cache can do.
- Socket-token delta. `ANTHROPIC_UNIX_SOCKET` is 66/73/76. Signature
  counts for 2.1.318 → 2.1.326, partitioned by the byte before the token
  so they sum to the totals: property reads
  (`.ANTHROPIC_UNIX_SOCKET`) 50/55, quoted spellings 10/10, all other
  contexts (bare string-table entries and name-list spellings) 13/11;
  50+10+13=73 and 55+10+11=76, so the net change is five more property
  reads and two fewer other-context spellings. Finer signatures overlap
  those classes and are unchanged (key position 2/2 is a subset of the
  quoted spellings; an `=` after the token, 6/6, cuts across both). The exception site
  itself is fingerprint-identical (above), the `"unix_socket"` transport
  classifications are unchanged (8/8/8), and every 2.1.326 property-read
  context read for this revision is a presence check that classifies the
  transport or disables a feature when the socket is set (including the
  new feedback/compliance disqualifier read, which returns false when the
  socket is set). The `"tunnel"` literal grows 3/3/5; the two new contexts
  are an unrelated HTTP CONNECT decision (`"tunnel"` vs `"direct"` /
  `"refuse"`) in proxy handling whose proxy selectors the launcher refuses
  and removes. The claim rests on the quoted eligibility code, not on
  attributing the five net-new property reads; see "Method and limits".

The policy-limits identifier surface is otherwise unchanged except one
added event name (`policyLimitsRefreshed`, 0/0/4), and the `Policy limits:`
log surface gains only two strings in 2.1.326, one of which is stricter
adoption ("not adopting the stored policy file until this hosted
process's own fetch answers. What the file denies still applies."). As
the README already states, this is a static observation, not a runtime
connection witness.

## Claim 4: native-tail allowlist and dispatch surface

Option declarations were extracted as the quoted first argument of
`.option(...)` and `.addOption(new <Option>("…", …))`, across all commands:
220 distinct declaration strings in 2.1.316, 224 in 2.1.318, 231 in
2.1.326 (210, 214 and 220 distinct long-flag names). The counts include
three declarations per build constructed in a `let y = new <Option>(…)`
form and added conditionally (`--cowork`, `--messaging-socket-path`,
`--proactivity`), and exclude `--jq`: that flag has no declaration in any
build (it is a `gh` flag in mapping tables and call sites, plus an error
construction `new <Error>("--jq needs jq…")`, which is how the first
draft's extraction counted it). For every allowlisted option the set of
declaration strings is identical in all three builds (each string was
matched literally for this revision):

| Shape in `gatewayNativeOptions` | Options (declaration in all three builds) |
| --- | --- |
| none | `--print` (`-p, --print`), `--verbose`, `--include-hook-events`, `--include-partial-messages`, `--replay-user-messages`, `--dangerously-skip-permissions`, `--allow-dangerously-skip-permissions`, `--strict-mcp-config`, `--exclude-dynamic-system-prompt-sections`, `--no-session-persistence`, `--disable-slash-commands` |
| optional | `--debug` (`-d, --debug [filter]`; a bare `-d, --debug` declaration on another command exists in all three builds) |
| required | `--debug-file <path>`, `--output-format <format>`, `--input-format <format>`, `--json-schema <schema>`, `--permission-mode <mode>`, `--permission-prompt-tool <tool>`, `--thinking <mode>`, `--thinking-display <display>`, `--max-thinking-tokens <tokens>`, `--max-turns <turns>`, `--max-budget-usd <amount>`, `--system-prompt <prompt>`, `--system-prompt-file <file>`, `--append-system-prompt <prompt>`, `--append-system-prompt-file <file>`, `--model <model>`, `--fallback-model <model>`, `--effort <level>`, `--agent <agent>`, `--session-id <uuid>`, `--name` (`-n, --name <name>`) |
| variadic | `--allowedTools, --allowed-tools <tools...>`, `--disallowedTools, --disallowed-tools <tools...>`, `--tools <tools...>`, `--mcp-config <configs...>` (a `<config>` form on another command exists in all three builds), `--betas <betas...>` |

Short forms are likewise unchanged (`-p` none, `-d` optional, `-n`
required; the full short-option declaration set is identical). Two
non-shape notes: 2.1.326 adds a `--name <slug>` declaration on the
`worktree-hook` command (still a required value; the main `-n, --name
<name>` is unchanged), and the `--max-budget-usd` validation message
changes from "must be a positive number greater than 0" (2/2/0) to "must be
a number greater than 0" (6/6/8); the declaration and throw form are
unchanged. Only declaration shapes were compared: the value domains and
effects of allowlisted options were not examined in any build (see
"Method and limits").

New declared flag names in 2.1.326 that are not allowlisted. Dating
against 2.1.314 directly is not possible from the staged artifacts (no
2.1.314 tarball was provided), so "new" below is against the earliest
staged build, 2.1.316; every row is therefore also new since 2.1.314.
Options already declared in 2.1.316 cannot be dated against 2.1.314 this
way. All of these refuse in a gateway native tail (not allowlisted;
several sit on subcommands whose first word is already reserved). This is
the allowlist working as designed, not a contract change: claim 4b HOLDS,
with the dating caveat in the claim table.

| Flag (2.1.326 declaration) | New vs 2.1.318 | Where declared; what it selects |
| --- | --- | --- |
| `--managed-settings-in-initialize` (none) | Yes | Main program, hidden, SDK: read policy-tier managed settings from the `managedSettings` field of the initialize control request instead of `--managed-settings`. **Selects a settings source** |
| `--lead-verify-key <time.key>` | Yes | Main program, hidden teammate option: launch time plus the public key that checks the team lead's messages. Team-message trust, not a settings source, directory, stored session, plugin, transport or endpoint by itself |
| `--event <event>` (mandatory, `create`/`remove`) | Yes | Hidden `worktree-hook` command (runs WorktreeCreate/WorktreeRemove hooks, JSON out) |
| `--path <absolute>` | Yes | `worktree-hook`: real path of the worktree on remove. **Names a directory** (on that command) |
| `--trusted-folder <absolute>` | Yes | `worktree-hook`: current directory the host says a person trusted. **Names a trusted directory** (on that command) |
| `--discard` (none) | Yes | `plugin restore-set-aside`: delete set-aside plugin data instead of restoring it. **Plugins** |
| `--review-md <file>` | Yes | `ultrareview` hidden option (review file input) |
| `--list <file>`, `--output <file>`, `--max-cost <dollars>` | No (present in 2.1.318) | `ultrareview` hidden options |
| `--web` (none) | No (present in 2.1.318) | `mcp add`: add the server as a claude.ai connector instead of to local config. **Plugins/connectors, stored config** |

One declaration present in 2.1.316/2.1.318 is gone in 2.1.326:
`--registry <url>` on a plugin install command (binary token `--registry`
14/14/9; remaining occurrences are other contexts).

For scale: 2.1.326 declares 180 distinct long-flag names that are not on
the allowlist (220 − 40). The appendix lists them; only the names in the
table above are new against 2.1.316, the rest were already declared in
both grounded builds and already refuse.

### Eager scanner and `--managed-settings-in-initialize`: value-slot coverage

Besides its option declaration, the flag joins the eager scanner that runs
before the option parser (exports `eagerHasCliFlag`, `eagerFindCliArg`,
`lastValueSlot`, `eagerParseCliFlag`). In 2.1.326 those are `Hw`, `ttt`,
`n1e`, `$y`, all read directly for this revision:

- `ttt` walks argv, breaks at the first `--`, tests each token, then
  advances past `n1e` (the last value slot of the token just tested).
- `Hw(flag)` is `ttt` with exact equality — no `=value` form can honour it.
- `n1e` skips with three extracted sets: `l` (optional-value flags — skip
  the next token only when it does not look like an option), `i` (every
  value-taking flag — skip one slot), and `K0n` (variadic — skip the first
  slot and each following non-option token). Option-looking means more
  than one character starting with `-`, as in the launcher.
- `$y`/`d` (eager value parse) use the same `n1e` skipping and handle
  `flag=value` forms.

Every slot the launcher passes unchecked is a slot this scanner also
skips. The launcher skips exactly the token after each allowlisted
required or variadic option (and treats later variadic items that look
like options as options, which then refuse when not allowlisted). The
coverage, name by name against the extracted sets:

| Launcher allowlist value options | In scanner sets |
| --- | --- |
| required (21): `--debug-file`, `--output-format`, `--input-format`, `--json-schema`, `--permission-mode`, `--permission-prompt-tool`, `--thinking`, `--thinking-display`, `--max-thinking-tokens`, `--max-turns`, `--max-budget-usd`, `--system-prompt`, `--system-prompt-file`, `--append-system-prompt`, `--append-system-prompt-file`, `--model`, `--fallback-model`, `--effort`, `--agent`, `--session-id`, `--name` | Every one in `i` |
| required short: `-n` | In `i` |
| variadic (7): `--allowedTools`, `--allowed-tools`, `--disallowedTools`, `--disallowed-tools`, `--tools`, `--mcp-config`, `--betas` | Every one in `i` and in `K0n` |
| optional: `--debug`, `-d` | Both in `l`; an option-looking token after them is refused by the launcher as an option, so no slot is passed unchecked |

Position analysis for the token itself follows from the two mechanisms:
as the first value of a covered flag, both skip it; as a later variadic
item or after an optional or valueless option, the launcher refuses it as
a non-allowlisted option; in `--flag=<token>` inline form and after `--`,
the scanner's equality test and terminator break never honour it. The
flag's other eager consumer selects an early incompatibility error
("can't be combined with `--bg`, … or a background session command"),
which starts nothing. Value-slot coverage is therefore complete today.

The initialize request without the flag was also traced. The launch-time
reader is started as `u6i(stdin, W)` only when the parsed options hold
`awaitInitialize`, stream-json input, no SDK URL and piped stdin, where
`W = getOptionValue("managedSettingsInInitialize")===!0` — a parsed-option
value, so it can be true only when the option parser actually consumed
the flag as an option, which the allowlist refuses in every such position.
The parser extracts `request.managedSettings` only `if (W)`; otherwise
the parsed result carries no managed settings, and the apply step
(`setOptionValue("managedSettings", …)`) is reached only through that
flow. A later (re-sent) initialize in an ordinary session only compares
its `managedSettings` against the settings stored at launch to answer
`managed_settings_applied`; it applies nothing. So with the flag absent —
the only state a gateway launch can be in — an initialize carrying
`managedSettings` (possible because `--input-format` is allowlisted) does
not select a settings source.

The token is nevertheless reserved globally in the launcher change, on the
`--bg`/`--background` precedent (both are `eagerHasCliFlag` tokens). A
settings-source selector should not depend for its safety on the next
allowlist addition remembering to re-check a scanner set in a future
bundle; the reservation refuses the exact token in every position,
values and after `--` included, at the same cost as every other reserved
word (a prompt that is exactly that token refuses).

### Dispatch surface, by category

The launcher's reserved words are built from dispatch categories outside
the option parser. Each category was re-extracted for 2.1.316 and 2.1.326;
only additions matter, and each is dispositioned here:

| Category | Extraction | 2.1.326 result |
| --- | --- | --- |
| Command names | `.command("…")` literals | `+worktree-hook`, `+workspace-trust` (hidden top-level commands), `+restore-set-aside <plugin>` under the already-reserved `plugin`, `−drop-worktree-registrations` (kept reserved: the grounded builds dispatch it). The first two are reserved by the launcher change |
| Alias tables | `.alias("…")` literals and the plugin alias table (`plugin init/install/uninstall/prune/marketplace` entries with `aliases:[…]`) | Identical sets (`kill`, `plugins`, `rc`, `upgrade`; `new`, `i`, `remove`/`rm`, `autoremove`, `rm`) |
| Whole-argv includes | `process.argv.includes("…")` literals | Only `+--ignore-persisted-trust`. (`ke.argv.includes("cd")` on a parsed shell command is new and is not process-argv dispatch) |
| Whole-argv indexOf | `process.argv.indexOf("…")` reached via `argv.indexOf` literals | Identical (`--`, `--handle-uri`) |
| Positional equality | `process.argv[2]==="…"` literals | `+--ignore-persisted-trust` (the splice that pairs with the includes check), `+--preload` in this syntactic form (the word is already reserved) |
| Eager `startsWith` predicates | `ttt((x)=>x.startsWith("…"))` | Only `--sdk-url`, as before (refuses as an option; value position is scanner-skipped) |
| Eager flag/value call sites | `Hw("…")`, `$y("…")` literals in the defining chunk | `Hw`: `-p`, `--print`, `--bg`, `--background`, `--ax-screen-reader`, `--host-auth-is-org-sign-in`, `--init-only`, `--no-session-persistence`, `--restricted`, `--strict-mcp-config` — each is an allowlisted switch, already reserved, or a non-allowlisted option. `$y`: `--settings`, `--managed-settings`, `--setting-sources` and six non-source flags — value-aware through `n1e`, and each settings-source flag refuses as an option |
| Before-terminator getters | `lNt(token)` definition and call sites (`indexOf(token) < indexOf("--")`, no value skipping) | Only `--bare` and `--safe-mode`, exactly the launcher's before-terminator pair |
| First-operand modes and background verbs | First-operand finder (`Een`, same `n1e` walk), invocation classifiers (`isMcpServeInvocation`, `isPluginEvalInvocation`, `isRemoteControlInvocation`, `isAgentsJsonInvocation`, `isProjectVerbInvocation`, `NON_REPL_SUBCOMMANDS` — same export surface), background-verb set literal | The verb set `{attach,logs,stop,kill,respawn,rm}` is byte-identical in both builds. No new first-operand word appears beyond the new command names above; the mode words the launcher reserves are unchanged as a set |
| Other new argv walks | Literal `startsWith("--…")` additions outside the eager scanner | `--settings=` handling inside the `remote-control` subcommand's own argument filter (that mode's words are reserved, so it is unreachable from a gateway tail); a new `--add-dir` trust pre-scan that is value-index aware (it skips the same value indices, and `--add-dir` refuses as an option in every parser-consumed position); `--json`/`--config-file` in tool- and git-argument handling, not process argv |

### `--ignore-persisted-trust` and `CLAUDE_CODE_IGNORE_PERSISTED_TRUST`

Both positions slip through without the reservation, and both are now
reserved. In a value slot the launcher previously passed the token
unchecked; after `--` it stopped option checks and refused only reserved
words. The runtime meanwhile searches the whole argv twice, with no `--`
stop and no value-slot skipping: the entry path runs
`process.argv.includes("--ignore-persisted-trust")` and then sets
`process.env.CLAUDE_CODE_IGNORE_PERSISTED_TRUST="1"` (splicing the token
only when it sits at `argv[2]`), and the predicate
`envSet || process.argv.includes(token)` backs every later use. The
effect's direction is stricter: "no workspace trust record counts in this
process" (bundle prose, expanded in 2.1.326 to name the flag), and the
same predicate feeds the settings-source suppression helpers of claim 5c,
so a launch can start without the project settings source. Stricter is
not admitted by default under contract A: it is an unreviewed trust- and
settings-source mode selector, and reserving is the fail-closed treatment
given to `--safe-mode` and its peers.

The environment variable is not new: it occurs 10/10/11, and in 2.1.316
its reader is environment-only. The net-new occurrence is the entry-path
assignment above. No launcher change was needed for the variable itself,
and none is made: under the existing closed-namespace rule every
`CLAUDE_`/`ANTHROPIC_`/`CCR_` name outside the eight admitted ones refuses
when supplied (composed overlay, prompt overlay, MCP lookup or any checked
settings source) and is removed when inherited, and the final
composed-environment check refuses a survivor. A new test pins all four
supplied layers, the inherited removal, and an admitted control for this
exact name.

## Claim 5: managed-home files and `CLAUDE_CONFIG_DIR`

What holds, read in the tier-to-path function itself for this revision:
the user-settings file-name map is identical
(`{default:"settings.json", cowork:"cowork_settings.json"}`), selected by
the same condition (`coworkPlugins || CLAUDE_CODE_USE_COWORK_PLUGINS`), and
the `userSettings` branch returns `join(globalConfigDir, name)` unchanged
in shape. The staged-copy code copies `settings.json`,
`cowork_settings.json`, `.claude.json` and `.config.json` the same way.
The global config path is identical in form:
`` join(process.env.CLAUDE_CONFIG_DIR || <home>, `.claude${suffix}.json`) ``,
and the legacy path is `join(configHome, ".config.json")` in both the
credential resolver and the chooser.

`CLAUDE_CONFIG_DIR` grows 97/97/112 without a new home-resolution site:
fallback joins `CLAUDE_CONFIG_DIR||…` are 4/4/4 and direct
`process.env.CLAUDE_CONFIG_DIR` reads 18/18/18. The five genuinely new
quoted contexts are a memory/path environment list, a task manifest `env`
reads declaration, a memory-default predicate, and two host task
environment reads (`env.get("CLAUDE_CONFIG_DIR")`, one of which scans the
settings file names below); the rest of the delta is propagation and
scrub lists, export re-exports and one telemetry classification, all with
2.1.318 twins. `CLAUDE_SECURESTORAGE_CONFIG_DIR` handling, including its
`.normalize("NFC")` fallback to the config home, is fingerprint-identical
(14/14/17; the growth is list and profile plumbing). `ANTHROPIC_CONFIG_DIR`
(21/21/37) keeps the same resolver body — read in both builds — and its
new reads are profile, fleet and background-dispatch plumbing;
`SELF_HOSTED_RUNNER_HOST_CONFIG_DIR` is 9/9/10. Every one of these names
is in the launcher's closed namespaces or runner-control prefixes: each
supplied value refuses and each inherited value is removed, so none can
move a gateway child's managed home.

`settings.json` grows 453/467/502, mostly embedded documentation:
`.claude/settings.json` path mentions are 202/211/227 and
`managed-settings.json` is flat at 58/58/57. Quoted code literals are
42/45/54. Every net-new quoted site in 2.1.326 is attributed here;
pairing is by surrounding-context fingerprint modulo renames, and each
2.1.318 site has a 2.1.326 twin except the sites below.

- The adjacent-literal change is one new list,
  `["settings.json","cowork_settings.json"]` in a host task scan that
  reads a task manifest's declared `CLAUDE_CONFIG_DIR` and classifies
  the two user settings paths under it as sensitive file accesses (the
  `cowork_settings.json` 4/4/7 delta is that scan, a subdirectory
  sensitive-file list, and their shared context). It classifies paths;
  it reads no settings content and applies no `env`.
- One new `join(…, "settings.json")` site is the plugin validator's
  per-directory check (2.1.326 `jo(e)`): it reads
  `<plugin root>/settings.json` for the directory being validated and
  returns at most two deprecation notes, for the `agent` and
  `subagentStatusLine` keys. It produces a validation result; nothing
  is applied. The plugin loader that does consume that file (present in
  both builds) parses it against a schema admitting exactly those two
  keys (`["agent","subagentStatusLine"]` in both builds), so an `env`
  member in a plugin's `settings.json` is not read as settings in
  either build.
- The other new `join` site only formats a path for a diagnostic in
  the self-hosted runner (`[runner:stuck] write <dir>/settings.json`).
  The write it names seeds the runner's per-session config directory
  with a document the runner constructs itself,
  `{"skipWebFetchPreflight":true}`, taken from one boolean of the
  operator snapshot's staged `settings.json`
  (`files.get("settings.json")`, the remaining new quoted site; only
  that boolean is read). No `env` is copied or applied, and the path
  runs only while provisioning a hosted session, a mode the gateway
  tail cannot select.
- The new directory walk is in the added-folder plugin collection
  (2.1.326 `z()`, beside `Rye()`; 2.1.318 twins `Are()`/`Noo()`). For
  each additional directory — `--add-dir` values and directories a
  checked settings source admits through
  `permissions.additionalDirectories` — it reads
  `<dir>/.claude/settings.json` and `settings.local.json` (the subset
  the enabled setting sources allow) and counts only their
  `extraKnownMarketplaces` keys and `enabledPlugins` values, to warn
  that entries from an untrusted added directory are ignored. It skips
  the `settings.json` entry when the file's directory physically
  resolves to the config home, so the user's own settings file is not
  double-counted. The collectors that do apply something from those
  files read the same two members in both builds and nothing else; no
  `env` is read or applied from an added folder's settings, and
  2.1.326 only adds trust and download-folder filtering before the
  same two members are honoured.
- Two further new occurrences are the fallback label
  `??"settings.json"` when formatting the user-settings path in
  background-daemon error messages. Display only.

So in 2.1.326 the files whose `env` the runtime applies are exactly the
tier sources the guard already reads: the user `settings.json` and
`cowork_settings.json`, the one selected global file (claim 5b), every
project/local `.claude/settings.json` and `settings.local.json` the
guard walks, the managed file and drop-ins, and an explicit settings
input, which the gateway tail refuses. No new source applies `env`, and
no new writer to the user's `settings.json` or
`cowork_settings.json` was found in the extracts. A hypothetical new
writer would in any case land inside the existing residuals: those
files are read-only regular files under contract A, file modes are
same-uid-erasable, and a settings change after the boundary check is
the documented mid-session reroute.

### 5b. The legacy global-file chooser: case table

Both choosers and the launcher's selection were read in full. 2.1.316
(`y_()`, identical in 2.1.318): legacy wins when it stats as a file with
`size>0 || (writeBits && !existsSync(new))`, else the new file. 2.1.326
(`Q_()` returning `{file, unused?, legacy}`): enters on the same
`isFile() && (size>0 || writeBits)` gate, then stats the new file —
missing means legacy; a zero-size legacy still wins only when the new
file is an empty read-only file on Linux/WSL; a non-empty legacy wins over
a missing, non-file or empty new file, then checks the directory listing
for the exact legacy name (absent exact name means the new file wins and
the legacy path is only reported as probed), then a same device+inode
(non-Windows) test, else legacy wins with the new file reported `unused`.
New accessors `getUnusedGlobalClaudeFile` and
`getProbedLegacyGlobalConfigFile` (0/0/3 each) expose the extra results,
and new warning prose (the `.config.json` 9/9/11 delta) tells the person
an older build on another host can write a moved `.config.json` back. No
extracted consumer reads settings or `env` from the `unused` file; it is
reported, warned about, and left alone.

Reachable states in a contract-A managed home on darwin
(`CLAUDE_CONFIG_DIR` set, both files in that directory; the legacy file,
where present, is a regular file with no write bit — the launch refuses
otherwise; L = legacy `.config.json`, N = `.claude.json`):

| L \ N | absent | empty | non-empty file | non-file |
| --- | --- | --- | --- | --- |
| absent | all: N (not yet written) | all: N | all: N | all: N (326 falls through; the guard refuses an unreadable selected file) |
| empty, read-only | all: N | all: N | all: N | all: N |
| non-empty, read-only | all: L | all: L | 316/318: L; 326: L (`unused` N) — **except the case-variant row below** | all: L |

Writable-legacy rows cannot be admitted (contract A refuses the launch),
and agree where an `env` could differ in any case: an empty writable L is
chosen by all three only when N is absent (326 adds the Linux/WSL
empty-read-only-N tie-break, where L is empty and carries no `env`), and a
non-empty L follows the read-only rows. A hard-linked pair (same inode)
reads identical bytes under either name. The guard today implements the
2.1.316 rule exactly (same stat-follows-links, same `0o222` mask the
bundle spells `&146`), so:

- For 2.1.316/2.1.318 the guard already reads the `env` of the file the
  runtime chooses in every state. The `env` question is **not** a
  pre-existing gap in the grounded builds.
- For 2.1.326 there is exactly one reachable divergence: L non-empty,
  N non-empty regular and distinct, and the directory listing has no entry
  named exactly `.config.json` even though stat on that path succeeded —
  a case-variant entry (for example `.CONFIG.JSON`) on a case-insensitive
  filesystem. 2.1.326 then reads N while an unmodified guard checks L,
  and a route-bearing `env` in N would pass unchecked. The launcher
  change refuses exactly that state (both files regular, non-empty and
  distinct, exact legacy name unlisted), under the existing configuration
  code and with no file contents in the refusal. An ordinary home holding
  both files under their exact names is admitted exactly as before, and
  the existing tests that pin that keep passing.

### 5c. Project/local suppression guard

The tier function in 2.1.326, quoted for the project/local case, is:
compute the path as before, then
`if (Hs(n) && $le(r) && dht(r)) return;` — return no path. The 2.1.318
function has no such line; the `userSettings` and `policySettings`
branches are unchanged in shape. The helpers were traced structurally:
`Hs(n)` tests the project directory, `$le` tests whether some ancestor
path segment normalizes (lowercase, trailing dot/space and colon rules)
to a chunk constant, and `dht` tests whether a path prefix ending at such
a segment is a special directory, a `projects/<hash>`-shaped directory, or
holds download artifacts. The only string assignment to that constant in
the bundle is `"connector-downloads"`, beside the connector download code
of claim 2d; on the extracts the guard therefore suppresses project and
local settings for a working directory inside a connector-downloads
folder. The leaf identity rests on a cross-chunk identifier and two
helpers (`Fle()`, part of the artifact test) were not fully resolved, so
the exact predicate remains CANNOT TELL in that detail.

For the gateway the uncertainty is covered without it: syntactically the
guard can only return the previously computed path or no path — it never
substitutes another file — and the launcher already refuses a
route-bearing `env` in every `.claude/settings.json` and
`settings.local.json` from the project root to the filesystem root, along
both the given and the physical path. Any project/local file the runtime
can still read after suppression was already checked; a suppressed file
is one fewer settings source.

## Environment selectors (diff)

The README's selector sentence assumes the launcher knows every
endpoint, socket, first-party and client-data selector. Two inventories
bound what 2.1.326 adds:

- Namespace tokens. Every distinct `CLAUDE_`/`ANTHROPIC_`/`CCR_` token in
  the bundle text: 1,160 in 2.1.316 → 1,278 in 2.1.326 (125 raw additions,
  7 removals; a handful of the additions are identifier fragments split by
  templates, for example trailing-underscore pieces). Selector-shaped
  additions include `CLAUDE_AI_URL`, `CLAUDE_CODE_NOTIFICATIONS_HOST`,
  `CLAUDE_CODE_HOST_OWNED_CONFIG_DIR`, `CLAUDE_CODE_HOST_OWNED_SPAWN_ENV`,
  `CLAUDE_CODE_PLUGIN_REMOTE_MCP_REACHABLE_HOSTS_HINT`,
  `CLAUDE_CODE_PLUGIN_MCPB_DIR`, `CLAUDE_CODE_CHILD_NOTES_DIR`,
  `CLAUDE_CODE_AUTOMOUNT_ROOTS`, the
  `CLAUDE_CODE_USER_DELEGATED_SESSION_*` provider/model set,
  `CLAUDE_BG_GATEWAY_ROUTED`, `CCR_SEED_BUNDLE_PATH` and
  `CCR_AGENT_PROXY_MAX_BUSY_TUNNELS`. None of the 125 is among the eight
  admitted namespace names, so the existing rule refuses each one when
  supplied (any layer) and removes each when inherited, with no per-name
  code change. `CLAUDE_CODE_IGNORE_PERSISTED_TRUST` is not an addition: it
  is present in all three builds (see claim 4).
- Outside the namespaces. Distinct `process.env.<NAME>` literal reads:
  427 → 440. Additions beyond namespace names are
  `SELF_HOSTED_RUNNER_USE_ANTHROPIC_GH` (covered by the
  `SELF_HOSTED_RUNNER_` control prefix, which refuses supplied and removes
  inherited), `TSC_WATCHFILE`/`TSC_WATCHDIRECTORY`/`TSC_NONPOLLING_WATCHER`,
  `TEMP`, `TMP`, `TZ` (ordinary tool environment), and
  `NODE_INSPECTOR_IPC`/`VSCODE_INSPECTOR_OPTIONS`, which occur once each,
  only OR-ed into a `debugMode` detection boolean in the bundled compiler
  tooling; they open no listener on the extracts and need no disposition
  beyond this note. Quoted selector-suffix names (`*_URL`, `*_SOCKET`,
  `*_ENDPOINT`, `*_PROXY`, `*_HOST`) add exactly two:
  `CLAUDE_CODE_NOTIFICATIONS_HOST` (namespace) and
  `OTEL_RESOURCE_INCLUDE_HOST` (`OTEL_` control prefix).

The dominant environment access in the bundle is through lazy getters,
not `process.env.` literals, so the second inventory is a floor for
outside-namespace reads; the first is exhaustive for whole namespace
tokens spelled in the bundle text (a name constructed by concatenation
would evade both, as it would have in the grounded builds).

## Claim 6: every `downloads.claude.ai` path, and the model-catalog gate

An admitted 2.1.318 gateway launch without the catalog pair was
observed opening TLS directly to `downloads.claude.ai`. The build that
launch ran is the published 2.1.318 darwin-arm64 binary, SHA-256
`677a2ea7bf576cedd20e191d973914ed8a41796f53a63b1923381ddc4d5b5e7d`
(the grounded table above; recomputed from the staged tarball for this
revision, as were both other binary hashes). If a future observation
cannot name the hash it ran, the attribution below does not transfer
to it. This claim classifies every occurrence of the host in the
2.1.318 and 2.1.326 binaries, identifies each fetch path's gates, and
evaluates them under the gateway's composed conditions:
`ANTHROPIC_BASE_URL` the registration-owned loopback base (not
first-party), `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`,
`CLAUDE_CODE_MODEL_CATALOG=off`, and `DISABLE_AUTOUPDATER=1` where an
operator sets it (the launcher does not; where that matters it is
stated). It is a static classification; no HOLDS here witnesses a
connection.

### Occurrence classification

Raw byte occurrences are 30 in 2.1.318 and 29 in 2.1.326 (2.1.316 also
has 30; it was not reclassified line by line). Eleven per build sit in
the embedded bytecode string table and duplicate literals whose code
sits in the bundled-JavaScript region; they are not separate paths.
Grouped by the code or data that carries them (counts are raw
occurrences, string-table copies included; 2.1.318/2.1.326):

| Class | What it is | Occurrences |
| --- | --- | ---: |
| Catalog fetch constant (`…/model-catalog/v1/catalog.json`) | The published-catalog fetch's default URL in code. (The literal full URL occurs 3/3/3 in total: this constant's two copies plus one spelling inside the desktop text below.) | 2/2 |
| Catalog schema URL (`…/model-catalog/v1/schema.json`) | The `$schema` member of the catalog document bundled into the binary as data; never fetched | 2/2 |
| Updater releases base (`…/claude-code-releases`) | Two code constants: the auto-update version pointer fetch, and the install/update path's channel, manifest (with `manifest.json.raw-sig.json` sidecar) and binary downloads | 3/3 |
| Official-marketplace base (`…/claude-code-releases/plugins/claude-plugins-official`) | The marketplace fetch: `latest` pointer, then the versioned zip | 2/2 |
| Marketplace telemetry host literal | The `host` field of the `tengu_plugin_remote_fetch` event the marketplace fetch emits; not a fetch | 2/2 |
| Downloads-client guard text | The error the CDN-only HTTP client throws for a non-CDN URL; the client itself is the direct-connection mechanism the three fetches above share | 2/2 |
| Client-data error text | Three message strings of the `--client-data-url` signed-document loader, whose URL must name this host | 6/6 |
| Desktop egress/metadata text | The `VM_BUNDLE_HOST` constant and the desktop app's configuration and egress-service descriptions (auto-update, model catalog, catalog-URL mirror, including one spelling of the catalog URL), compiled into the CLI binary; no CLI fetch reads them | 9/9 |
| Release-note sentence | "load model settings from a signed file on downloads.claude.ai; set CLAUDE_CODE_MODEL_CATALOG=off to turn this off", in the bundled changelog | 1/0 |
| Health-check instruction text | Prose telling the assistant which version URL to read with its tools, and to skip the lookup when nonessential traffic is disabled; not a runtime fetch | 1/1 |

So exactly four code paths in each build can open a connection to the
host: the published model catalog, the updater, the official
marketplace, and the client-data document. (A second catalog, the
account-served one, is fetched through the API client from
`/api/model_selector/…`, i.e. through `ANTHROPIC_BASE_URL`; it never
names this host.) Each is gated as follows; helper identifications are
by unique body fingerprint in the shared chunks, read in both builds.

### 6a. The catalog gate, predicate by predicate — HOLDS

The gate (2.1.316 `vUt`, 2.1.318 `c6t`, 2.1.326 `Fan`, token-identical
modulo renames) is, in order: off-spelling test on
`CLAUDE_CODE_MODEL_CATALOG` → `"env_off"`; bare mode → `"bare"`;
essential-traffic test → `"essential_traffic"`; `ANTHROPIC_UNIX_SOCKET`
set → `"unix_socket"`; first-party test → `"not_first_party"`; account
scope; `allow_model_catalog` policy → `"policy"`; else null.

- The off-spelling test reads a boolean as its negation and otherwise
  matches the lower-cased, trimmed value against
  `["0","false","no","off"]`. The gateway's `off` trips it. The served
  catalog's mode function maps any non-null gate result to `"off"`.
- The essential-traffic test is the presence of
  `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC` in the process
  environment (the same helper the artifact and telemetry gates use,
  returning their `"essential_traffic_only"` reasons). The gateway
  owns that pair at `1`, so it trips independently of `env_off`.
- The first-party test is: provider `firstParty` (no `CLAUDE_CODE_USE_*`
  selector and no host-held gateway credential) AND (the client-data
  first-party assumption of claim 3, or no `ANTHROPIC_BASE_URL`, or a
  base whose host is exactly `api.anthropic.com`). The loopback
  gateway base fails it, so the *served* catalog is off a third way.
- The *published* catalog — the one fetched from this host — passes
  the gate result through a translator (2.1.318 `z4n`/`y`/`w`, 2.1.326
  twins read here). `"env_off"`, `"bare"`, `"essential_traffic"`,
  `"unix_socket"` and `"policy"` pass through as off reasons.
  `"essential_traffic"` is re-run with that one test skipped only when
  the configured catalog URL — the managed source, else
  `CLAUDE_CODE_MODEL_CATALOG_URL` — parses as a `file:` URL; beside the
  owned `off` the re-run still returns `"env_off"` first, and under
  the gateway there is no such URL at all (below), so the bypass
  cannot fire. `"not_first_party"` is translated by provider:
  a host-held `gateway` provider yields `"cloud_gateway"` (off); the
  gateway's plain first-party-with-custom-base shape yields
  `"gateway_flag_off"` while the `tengu_delegated_quail` published
  mode is off (its compiled default is `{mode:"off"}`), and
  `"release_off"` behind that, because the release predicate it
  consults reads a compile-time constant that is `false` with no
  writer in either build. Bedrock/Vertex/Foundry/Mantle providers —
  unreachable under the gateway, whose route selectors refuse — are
  the shapes that translation exists to admit.
- Both network consumers (the start-up primary path, timed at start-up
  as `model_catalog_decision_ms`, and the shadow-compare path) return
  before constructing a request unless that translation is null; the
  fetch itself then goes through the CDN-only client directly to the
  catalog URL and its `.raw-sig.json` sidecar.

Under the gateway's composed environment the published fetch is
therefore off three independent ways, in 2.1.316, 2.1.318 and 2.1.326
alike. That corrects what main previously said: the fetch *is* gated
by the nonessential-traffic switch, and `CLAUDE_CODE_MODEL_CATALOG=off`
is kept as the first, unconditional test and as the switch the
runtime itself refuses to let project settings reopen ("a project's
settings can only turn the model catalog off"); the gateway is
stricter still, owning the pair outright. The comment in
`gateway.go` and the README's contract-A sentence are corrected in
this change to say so.

The document URL can be redirected by `CLAUDE_CODE_MODEL_CATALOG_URL`
(4/4/4; a mirror override, `file:` included). That name is not
admitted by the gateway: supplied in any layer it refuses under the
existing closed `CLAUDE_` namespace rule, and an inherited value is
removed. Beside the owned `off` it could not fetch in any case —
`env_off` precedes the `file:` bypass — so the refusal is defence in
depth that keeps the selector closed if the pair ever changes, not a
second switch. This is pinned by `TestPrepareGatewayModelCatalogURLEnv`
(plant G7, whose comment now says the same); it needs no production
change.

### The other three fetches under the gateway conditions

- **Updater.** The background version-pointer fetch returns null
  immediately under the essential-traffic test (both builds), and the
  auto-update checks return earlier still under the shared
  disabled-reason helper: `DISABLE_UPDATES`, `DISABLE_AUTOUPDATER`
  read as true, nonessential traffic disabled, or `autoUpdates`
  config — so the nonessential pair alone suffices, `DISABLE_AUTOUPDATER=1`
  or not. The manifest, sidecar and binary downloads are reached only
  from an update the check found, or from the `update`/`install`
  commands, which are reserved words in a gateway tail. Not reachable.
- **Client-data document.** Loading starts only from
  `CLAUDE_CODE_CLIENT_DATA_URL` or `--client-data-url`; the variable
  refuses when supplied and is removed when inherited, and the flag
  is not allowlisted (claim 3 covers its first-party writer). Not
  reachable.
- **Official marketplace.** The fetch (`latest`, then the zip) has
  three callers, identical in both builds (and present already in
  2.1.316): the bulk and single marketplace refreshes behind the
  `plugin` command (a reserved word, so unreachable from a gateway
  tail; the install-time refresh variants are additionally behind the
  essential-traffic test or the auto-updater disabled reason), and
  the one-shot official-marketplace auto-install. The auto-install is
  driven by an interactive-session setup effect and checks none of
  the four gateway conditions: its gates are its own
  attempted/installed/backoff state in the global file, its
  `CLAUDE_CODE_DISABLE_OFFICIAL_MARKETPLACE_AUTOINSTALL` kill switch
  (which the gateway removes when inherited and refuses when
  supplied, so a gateway launch cannot carry it), already-installed,
  and the enterprise marketplace allowlist (no list applies inside
  the boundary, claim 3). In a home where it has not yet run, an
  interactive gateway launch can therefore still fetch
  `…/claude-plugins-official/latest` and the zip directly from this
  host. Whether the setup effect also mounts in print launches could
  not be settled statically. This is the one path to the host that
  remains reachable under the gateway's conditions; it is unchanged
  in 2.1.326 and is not closed by this grounding.

### 6b. The observed connection — CANNOT TELL

The observed 2.1.318 launch already ran with the other three owned
pairs, `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1` included, so on
the extracts its catalog gate returned `"essential_traffic"` and no
catalog document was fetched: the observed connection was not the
model-catalog fetch, and adding the pair cannot be credited with
stopping it. Of the remaining paths, the marketplace auto-install is
the only one a gateway launch of that shape could have taken, but
which path actually fired — or whether the connection was something
this static pass cannot see — is exactly what the runtime witness
exists to decide, ideally recording the request path (marketplace
`latest`/zip versus `model-catalog/v1/catalog.json`) and the binary
hash it ran. The README therefore keeps the witness as a required
step for every grounded build, 2.1.326 included, now naming the
marketplace auto-install beside connector downloads; the catalog pair
is defence the witness must still confirm, not a substitute for it.
Closing the auto-install path from the launcher (for example owning
its kill-switch variable as a fifth pair) is a separate decision this
record does not take: nothing in this grounding depends on it, and
the witness remains required either way.

## What this grounding required

Each item names the change, whether it is global or keyed to 2.1.326, and
its evidence. Global is preferred throughout: semantics are selected only
through the strict-tuple hash, so a 2.1.326-only branch would be new
build-conditional code, and every narrowing below is inert or unreachable
on the grounded builds.

| # | Change | Scope | Code (internal/network) | Test | Plant |
| --- | --- | --- | --- | --- | --- |
| 1 | Reserve `--ignore-persisted-trust` (whole argv, both sides of `--`) | Global | `claudeReservedArgs` in `native_args.go` | `TestPrepareGateway326DispatchWords` | G1 |
| 2 | Reserve `worktree-hook`, `workspace-trust` | Global | `claudeReservedArgs` in `native_args.go` | `TestPrepareGateway326DispatchWords` | G2, G2b |
| 3 | Reserve `--managed-settings-in-initialize` (coverage proven in claim 4; `--bg` precedent) | Global | `claudeReservedArgs` in `native_args.go` | `TestPrepareGateway326DispatchWords` | G3 |
| 4 | `CLAUDE_CODE_IGNORE_PERSISTED_TRUST`: supplied refuses, inherited removed (existing closed-namespace rule; no code change) | Global (existing) | `gatewayRefusedName` in `settings_guard.go` | `TestPrepareGatewayIgnorePersistedTrustEnv` (new pin) | G4 (admits the name) |
| 5 | Refuse the build-divergent global-file state (both non-empty, distinct, exact legacy name unlisted) | Global (triggers only where choice would be build-dependent) | `selectedGlobalFile`, `exactLegacyListed` in `settings_guard.go` | `TestPrepareGatewayGlobalFileCaseVariant`, `TestPrepareGatewayGlobalFile326Cases`, `TestExactLegacyListedReal` (the production lister, not a stub) | G5, G6, G8 |
| 6 | Connector: no cache narrowing; README bounds the no-literal-consumer sentence and names connector downloads beside the runtime witness (claim 2d) | Text only | — | Existing policy-cache vectors keep passing | — |
| 7 | `upcoming_minimum_version` and every new restriction key: keep refusing (existing closed subset) | Global (existing) | `policyCachePath` in `policy_cache.go` (unchanged behaviour) | Existing policy-cache vectors | — |
| 8 | Model catalog (claim 6a): the gateway sets and requires `CLAUDE_CODE_MODEL_CATALOG=off`; already landed independently of this change, relied on here, not duplicated; its `gateway.go` comment is corrected there (the fetch is also gated by the nonessential-traffic switch) | Global (existing) | `gatewayClaudeNames`, the registration patch and `gatewayCompositionCheck` in `gateway.go` | `TestPrepareGatewayBindsQualifiedClaude`, `TestGatewayComposedEnvironmentBoundary`, the settings-conflict catalog case | — |
| 9 | `CLAUDE_CODE_MODEL_CATALOG_URL`: supplied refuses, inherited removed (existing closed-namespace rule; no code change) | Global (existing) | `gatewayRefusedName` in `settings_guard.go` | `TestPrepareGatewayModelCatalogURLEnv` (new pin) | G7 (admits the name) |

Refusals reuse the existing codes: native-tail refusals are the
configuration-conflict code the tail already returns, the legacy-listing
refusal is the same code at the settings boundary, and contract-A cache
refusals stay `network_scope_unsupported`. No new refusal code is added.
Plants live in `scripts/gateway-claude326-controls.sh` (`make
gateway-claude326-controls`): each plant weakens one gate in a scratch
copy, its named test must fail there after passing unmodified, and the
working tree is never mutated.

What a gate owner must not conclude from this record:

- It qualifies no build by itself. A proxied run still needs its own
  qualified strict-tuple hash entry naming this record, under the landed
  launcher change, plus the runtime witness the README requires.
- No HOLDS here is a connection witness, including claim 2d: it is a
  static reachability proof over minified code, and the observed process
  must still make no direct Anthropic connection, connector downloads
  included.
- Claim 2d does not say the admitted cache values scope connector egress
  in an eligible session; that behaviour is unchanged and documented.
- Allowlisted options were compared by declaration shape only, not by
  value domain or effect.

## Method and limits

Tarballs were listed (`tar -t`) and extracted (`tar -x`) as data;
binaries were identified with `file`, hashed with `shasum`, and searched
with `strings`, `grep -a -o -F/-E`, `wc`, `sort`, `comm` and `diff`.
Nothing from the tarballs was executed or loaded, and no network was
used. Counts are raw literal occurrences in the Mach-O binary and
therefore include documentation and message strings where a token also
appears there. Each first-draft count was recomputed for this revision
and reproduces; multi-pattern passes were used only where patterns cannot
overlap, after verifying the overlap behaviour on the overlapping cases
(`settings.json` inside longer file names, the `=!0`/`=!1` writers inside
`assumesFirstPartyBaseUrl`).

Limits that bound the verdicts:

- Findings are limited to statically visible bundled code. Minified
  identifiers are reused across chunks, so a helper identified only by a
  shared name (the `LL()` outcome fingerprint in claim 2d, the
  `"connector-downloads"` constant in claim 5c) is an identification by
  unique string fingerprint and neighbouring literals, not by a symbol
  table. Where that matters, the text says so.
- Dynamic behaviour (feature-flag state beyond the quoted checks, sign-in
  state, actual connections) is out of scope; no verdict witnesses a
  running process.
- Two count deltas are accounted at signature and role level, not by
  pairing every occurrence with a context: `ANTHROPIC_UNIX_SOCKET`
  (claim 3) and `CLAUDE_CONFIG_DIR` (claim 5). For `settings.json` and
  `downloads.claude.ai` every net-new (claim 5) or every raw (claim 6)
  occurrence in 2.1.318/2.1.326 is attributed above; pairing is by
  surrounding-context fingerprint modulo renames, and 2.1.316 was
  spot-checked, not reclassified line by line. Minified renames make
  exact context diffs report almost every context as new, so the
  remaining deltas quote the load-bearing sites directly and classify
  the rest.
- Allowlisted option value domains and effects were not examined
  (claim 4a is declaration shapes only), exactly as in the checks that
  grounded 2.1.316/2.1.318.
- Dispatch and environment inventories extract literal spellings. A word
  or name the bundle constructs at runtime (concatenation, decoding)
  would evade them, in 2.1.326 as in the grounded builds.
- A 2.1.314 artifact was not staged, so "new since 2.1.314" for claim 4 is
  established against 2.1.316 as the earliest staged proxy, and the
  2.1.319 connector-consumer history in claim 2b rests on the earlier
  report, not on a re-count here.

## Appendix: 2.1.326 declared long flags not on the allowlist (180)

Extracted as described under claim 4 (distinct long-flag names from
option declarations across all commands, minus `gatewayNativeOptions`).
`--jq` is not declared in any build and is not listed: it is a `gh` flag
in mapping tables and call sites, plus the message string
`--jq needs jq, which is not installed…`.

`--ablation --accept-command --add-dir --advisor --agent-color
--agent-id --agent-name --agent-type --agents
--agents-view-if-allowed --all --allow-other-models
--allow-real-servers --allow-tools
--append-subagent-system-prompt --append-subagent-system-prompt-file
--attach-serve --author --author-email --autocompact --available
--await-claim --await-initialize --ax-screen-reader --background
--bare --bg --brief --callback-port --case --channels --chrome
--claudeai --client-data-url --client-id --client-secret --cloud
--concurrency --config --console --continue --correlation-id
--cowork --cwd --dangerously-load-development-channels --data-size
--debug-to-stderr --deep-link-cwd-b64 --deep-link-last-fetch
--deep-link-origin --deep-link-repo --description --desktop
--disallow-tools --discard --dry-run --email --enable-auth-status
--enable-auto-mode --env --environment --eval-dir --event
--fast-mode --file --force --fork-session --forward-home-settings
--forward-subagent-text --from-link --from-pr --gateway-json
--header --host-auth-is-org-sign-in --id-token --ide
--include-unsupported-models --inherit-permission-mode --init
--init-only --interactive --interview --json --judge-model
--keep-data --keep-temp --label --lead-verify-key --list
--maintenance --managed-settings --managed-settings-in-initialize
--marketplace --max-cost --max-cost-usd --message
--messaging-socket-path --mocks --no-browser --no-chrome
--no-post --no-publish --no-scaffold --on-branch --output
--output-dir --parent-session-id --path --permission-prompts
--plan-mode-instructions --plan-mode-required --plugin-dir
--plugin-dir-no-mcp --plugin-url --pool --port --post --prefill
--prefill-b64 --proactivity --profile --project-config-root
--prompt-suggestions --prune --publish-report --push --rc --ref
--remote --remote-control --remote-control-session-name-prefix
--replace --reply-on-resume --report --resolve-only --restricted
--result-format --resume --resume-drops-turn --resume-session-at
--review-md --rewind-files --runner-declares --runs --safe-mode
--scaffold --scope --sdk-url --session-began-as-copy
--session-mirror --session-tunnel --setting-sources --settings
--sparse --sso --status --strict --system-prompt-snapshot --tag
--task-budget --team-name --teammate-mode --teleport --text
--threshold --timeout --tmux --transport --trust-plugin
--trust-workspace --trusted-folder --values-stdin --watch-artifact
--watch-artifact-no-autoreact --web --with --workload --worktree
--xaa --yes`
