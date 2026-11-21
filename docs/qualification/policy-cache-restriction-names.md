# Policy-cache restriction names `allow_connector_file_uploads` and `allow_computer_use` (gateway admission contract A)

Read-only artifact analysis, 2026-11-21. It extends the closed subset of
`policy-limits.json` that the gateway cache check admits by two restriction
names, and records the check that each admission rests on. Method and
vocabulary are those of
[claude-code-2.1.326-darwin-arm64.md](claude-code-2.1.326-darwin-arm64.md),
called "the 2.1.326 record" below; its claims 1 to 3 are the base this
record builds on and are not repeated.

**Verdict: both names are admitted with either boolean, in the same entry
shape as the two names admitted before them.** In each grounded build the
cached value of either name changes nothing in a session inside the gateway
input boundary: such a session is not eligible for organisation policy
because of the base-URL test quoted below, and an ineligible session returns
before the cache file is read. `allow_computer_use` additionally occurs in
none of the builds. Admission is by exact name. A restriction name without
such a check stays refused, and no build is added to the grounded builds by
this record.

## Why the names are needed

The policy endpoint now returns four restriction entries where the cache
grounded on 2026-11-05 held two: `allow_connector_file_downloads`,
`allow_connector_file_uploads`, `allow_computer_use` and
`enforce_web_search_mcp_isolation`, each an object with one boolean
`allowed`. A session outside the gateway refreshes the cache of its managed
home with that shape, and the closed subset then refused every later gateway
launch from that home with
`server policy caches are not admitted: policy-limits.json at restrictions.*`.
A home used only through the gateway never refreshes its cache (it is
ineligible) and kept the two-name shape. Refusing an unexamined name was the
designed behaviour; this record supplies the examination.

## Artifacts

Published npm `@anthropic-ai/claude-code-darwin-arm64` tarballs, unpacked as
data and searched only; nothing was executed or loaded.

| Build | Tarball sha256 | Binary sha256 | Binary bytes | Status |
| --- | --- | --- | ---: | --- |
| 2.1.316 | `3515788bc9db6c52ef8edb776228ce647de1af710efab85d289782a872147baa` | `64c327e92b2c5e0f78c0a980c5a518121e1f0d140ca771ca5dd4488d1a2dcb3f` | 310,114,176 | grounded |
| 2.1.318 | `02a686b84bbd96a6fd33263770e784aa5a959c6287e669e298667f23e75a96ba` | `677a2ea7bf576cedd20e191d973914ed8a41796f53a63b1923381ddc4d5b5e7d` | 319,598,464 | grounded |
| 2.1.326 | `bda994b80919b5df9588b81c98b8f8a08aa4ca6ba605efda74679ed8571e8136` | `e214ad76dcc0e846b03be28ed1c2734aacd4e27acb527cb7f8a9e2bed644e837` | 353,037,664 | grounded |
| 2.1.327 | `c32373eba8c96c7671f0595eee9503a77fdabd4136aa87d9c389da86214e66ef` | `76d48205c926c8a6978cfc2171b631bcd01e20bbadd085638bb2ec57bea91c5b` | 354,515,104 | observed only |

The first three rows equal the 2.1.326 record's artifact table. 2.1.327 is
the build that produced the four-name cache; only its policy module was read
here, so it is **not** grounded: the 2.1.326 record's claims 1 and 4 to 6
(plist, native tail, settings paths, catalog fetch) were not checked for it.

Calibration: the literal counts of the 2.1.326 record's table were recomputed
for 2.1.326 with the same command form and reproduce (for example
`policy-limits.json` 11, `allow_connector_file_downloads` 5,
`enforce_web_search_mcp_isolation` 2, `compliance_taints` 42,
`custom_base_url` 29, `assumesFirstPartyBaseUrl=!0` 1), and the number of
policy-table names is 37, 39 and 43 as recorded there (44 in 2.1.327).

## The network-relevant claim: eligibility is false because of the base-URL test

Everything below depends on one function, the credential-independent
ineligibility test, and on the launcher's input boundary. Quoted per build:

| Build | Credential-independent ineligibility (quoted) |
| --- | --- |
| 2.1.316 | `function Xur(e={}){if(Ce()!=="firstParty")return"third_party_provider";if(!e.skipBaseUrlCheck&&!FD()&&!Ii())return"custom_base_url";return}` |
| 2.1.318 | `function Pvr(e={}){if(Te()!=="firstParty")return"third_party_provider";if(!e.skipBaseUrlCheck&&!lN()&&!ci())return"custom_base_url";return}` |
| 2.1.326 | `function xnr(e={}){if(ve()!=="firstParty")return"third_party_provider";if(!e.skipBaseUrlCheck&&!yB()&&!Ki())return"custom_base_url";return}` |
| 2.1.327 | `function oir(e={}){if(Te()!=="firstParty")return"third_party_provider";if(!e.skipBaseUrlCheck&&!L2()&&!qi())return"custom_base_url";return}` |

The second operand is the tunnel-socket exception (`ANTHROPIC_UNIX_SOCKET`
with a credential) and the third is the first-party base test, in 2.1.327:

```
function qi(){if(xV())return!0;return nc()}
function xV(){return s._CLAUDE_CODE_ASSUME_FIRST_PARTY_BASE_URL??hr().clientDataUrl.assumesFirstPartyBaseUrl}
function nc(){let e=process.env.ANTHROPIC_BASE_URL;if(!e)return!0;return fd(e)}
```

The other three builds have the same three functions modulo renames
(2.1.316 `Ii`/`e6`/`Ou`, 2.1.318 `ci`/`Qz`/`Bc`, 2.1.326 `Ki`/`wz`/`pc`).
Inside the gateway boundary `ANTHROPIC_BASE_URL` is the registration-owned
loopback endpoint, which is not a first-party host; the launcher refuses a
supplied socket, first-party-assumption or client-data selector and removes
an inherited one, and the native tail refuses `--client-data-url`. So the
function returns `"custom_base_url"`, and `isPolicyLimitsEligible`
(`Lh`/`Oy`/`xy`/`zy`, each `return <reason>()===void 0`) is false. This is
the 2.1.326 record's claim 3; the lines above re-read it in all four builds.

What eligibility gates, quoted for the cache read:

| Build | Cache read (quoted) |
| --- | --- |
| 2.1.316 | `function wE(){if(!Lh())return null;let e=a();if(e.sessionCache)return e.sessionCache;if(e.diskAdoptionSuppressed)return null;let n=B5t();if(n)return $5t(n),n;return null}` |
| 2.1.318 | `function bk(){if(!Oy())return null;let e=a();if(e.sessionCache)return e.sessionCache;if(e.diskAdoptionSuppressed)return null;let n=cen();if(n)return len(n),n;return null}` |
| 2.1.326 | `function yT(e=xy()){if(!e)return null;let n=a();if(n.sessionCache)return n.sessionCache;if(n.diskAdoptionSuppressed)return null;let i=Rvn();if(i)return Cvn(i),i;return null}` |
| 2.1.327 | `function tC(e=zy()){if(!e)return null;let n=a();if(n.sessionCache)return n.sessionCache;if(n.diskAdoptionSuppressed)return null;let i=eRn();if(i)return QAn(i),i;return null}` |

`B5t`/`cen`/`Rvn`/`eRn` (exported as `loadCachedResponse`) is the only
function that opens `policy-limits.json`. Its callers are the same four in
every build, each behind the eligibility test:

- the cache read above, after its ineligible return;
- a predicate of the form `eligible() && load()===null`;
- the prefetch, which starts `if(!eligible())return`;
- the fetch cycle, which returns `{source:"ineligible",hadCache:!1,…}` before
  its load call (2 occurrences of that literal in every build), and whose
  three error paths are the only callers of `staleFallbackSource()` (5
  occurrences of that name in every build).

An ineligible session therefore never reads the file, and no restriction
entry in it, of any name, reaches the evaluator.

## Literal occurrences

`grep -a -o -F`, one token per pass, in the extracted binary.

| Token | 2.1.316 | 2.1.318 | 2.1.326 | 2.1.327 |
| --- | ---: | ---: | ---: | ---: |
| `allow_computer_use` | 0 | 0 | 0 | 0 |
| `allow_computer` | 0 | 0 | 0 | 0 |
| `allow_connector_file_uploads` | 0 | 2 | 3 | 3 |
| `"allow_connector_file_uploads"` (quoted constant) | 0 | 0 | 1 | 1 |
| `allow_connector_file_downloads` | 0 | 0 | 5 | 6 |
| `enforce_web_search_mcp_isolation` | 2 | 2 | 2 | 2 |
| `policy-limits.json` | 11 | 11 | 11 | 11 |
| `Object.entries/keys/values(…restrictions)` | 6 | 6 | 6 | 6 |

The first two tokens also occur 0 times as UTF-16LE in 2.1.326 and 2.1.327
(the search form was verified against a planted UTF-16 control), and no
template or concatenation that builds an `allow_` name was found there.

## `allow_computer_use`

No build contains the name: it has no policy-table entry and no evaluator
call. The server populates it; the client does not know it. The only code
that can touch a name it does not know is the six generic enumerations over
the restriction record, identical in number in all four builds: the two
filters of the refused-body hint builder (`hintFromRefusedBody`), the
unrestrictive-body test, two log lines that print a count, and one telemetry
count of denied entries. None selects a route, a settings source or a
permission mode, and all of them operate on a body that was read from disk or
fetched, which an ineligible session never does. A hint recorded for this
name would also never be asked for, since nothing evaluates the name.

Either boolean is inert, inside and outside the gateway boundary, in the
builds examined.

## `allow_connector_file_uploads`

A real client policy from 2.1.318: policy-table entry
`{deniedUnder:["hipaa","zdr"],onCacheMiss:"deny",onFailOpenVerdict:"deny",label:"Sending files to claude.ai connectors",…}`.

| Build | Consumers |
| --- | --- |
| 2.1.316 | none (name absent) |
| 2.1.318 | policy-table entry and one bare string-table entry; no evaluator call |
| 2.1.326 | one constant; 3 calls of `policyDenyKind` with it, all in the file-argument check of a claude.ai connector tool call |
| 2.1.327 | one constant; 5 calls of `policyDenyKind` with it: the same three, one upload gate and one document handler |

`policyDenyKind` begins `if(<isPolicyAllowed>(e))return null`, and
`isPolicyAllowed` is the evaluator of the 2.1.326 record's claim 2b. Quoted
from 2.1.327 (`zy` eligible, `tC` cache read); 2.1.326 is token-identical
modulo renames, and 2.1.316/2.1.318 have the same body with the eligibility
test inlined:

```
function Tn(e){return!H(e)&&!ee(e)&&Ce(e)}
function Ce(e){let n=zy(),i=tC(n);if(BVa(e)){…}
  if(!i){if(Mpl.has(e)){if((n||zy())&&!(…))return!1;if($e.has(e)&&bt()&&!(e==="allow_product_feedback"&&XEe()))return!1}
    if(M(e,Zss()))return!1;if(air(e,n||void 0))return!1;
    let o=a();if(o.hintedDeniedKeys.length===0&&o.hintedTaints.length===0)return!0;if(!(n||zy()))return!0;…}
  …}
```

In an ineligible session `i` is null without a read, and the remaining
denials on that path do not consult the cache: `$e` is the single feedback
name, `M(e,Zss())` tests the session's latched compliance taints (a separate
taint store, not the cache document), and `air` returns false when
ineligible. The hinted lists are consulted only after
`if(!(n||zy()))return!0`, that is, only when eligible. The result for this
name is therefore the same for a cached `true`, a cached `false` and an
absent entry.

Outside the boundary the value acts as organisation policy does: `false`
denies sending files to claude.ai connectors. As for the two earlier names,
this admission is conditional on the boundary, not an equivalence in general.

Reachability is independent of the cache and is not claimed here. Sending a
file to a claude.ai connector needs a claude.ai sign-in and a configured
claude.ai connector, and its target is Anthropic's connector proxy, not the
API base. In a gateway session policy is never asked, so policy does not stop
it; whether such a transfer can occur in a contract-A launch is the same
question as for connector downloads and belongs to the README's written
residual and its runtime no-direct-connection witness. A cache entry, present
or absent, does not change the answer.

## What changed between 2.1.326 and 2.1.327 in the policy module

The module was compared after masking minified identifiers of up to five
characters. Three differences: one new policy-table name
(`allow_m365_file_references`, deny on cache miss, refused by the closed
subset); two renamed locals; and one disqualifier expression moved into its
own function with the same terms. The eligibility functions, the cache read
and the evaluator are unchanged.

## What the launcher admits after this record

- `restrictions` may hold any of four names, each exactly `{"allowed": <boolean>}`:
  `allow_connector_file_downloads`, `allow_connector_file_uploads`,
  `allow_computer_use`, `enforce_web_search_mcp_isolation`.
- Everything else is as before: any other restriction name, an entry of
  another shape, `defaults.remote_control_at_startup` other than `false`, any
  other default, a non-empty `compliance_taints`, an object
  `monitoring_notice`, any other root member, a non-empty
  `remote-settings.json`, and a companion that is not a regular file of at
  most 64 KiB all refuse with `network_scope_unsupported`.

## Method and limits

Tarballs were extracted as data; binaries were hashed with `shasum` and
searched with `strings`, `grep -a -o -F` and context reads of the `strings`
output. Nothing from the tarballs was executed or loaded.

- Findings are limited to statically visible bundled code and witness no
  running process and no connection.
- "No occurrence" covers the literal in 8-bit form in all four builds and in
  UTF-16LE form in 2.1.326 and 2.1.327. A name that the server delivers at
  run time and that is passed to the evaluator would still meet the
  ineligible return quoted above.
- For 2.1.316 and 2.1.318 the eligibility function, the first-party base
  test, the cache read, the evaluator and the callers of the disk read were
  read; the remaining links of the chain are the 2.1.326 record's claim 3.
- `ANTHROPIC_UNIX_SOCKET` occurs 76 times in 2.1.326 and 104 in 2.1.327; the
  new occurrences were not attributed. The eligibility function reads the
  tunnel-socket exception the same way in both, and the launcher refuses and
  removes that selector.
- A build or input-boundary change needs this check repeated for each
  admitted name, as the README states for the earlier two.
