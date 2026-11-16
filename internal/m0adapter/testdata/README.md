# m0adapter fixtures

These four JSON files are verbatim copies of the inert documentary fixtures of
the DRAFT `swarma-dispatcher m0-executor-entry/1` proposal:

- source repository: `github.com/relux-works/swarma-dispatcher`
- source commit: `4fbf13a72a85a3b295e44115cbeffa49a90a4fb2` (`4fbf13a7`)
- upstream fixture README SHA-256: `32f4a092a1df6fc5b7e260153903fe5fd7ae06dc421201c2211626c9e1206aea`
  (verified separately; this file remains the launcher's provenance README)
- source directory: `fixtures/m0-executor-entry/`

| File | SHA-256 |
| --- | --- |
| `entry.specimen.json` | `16b898164b1b4bd0568a37bf71cbdf2625dc89f45544730866793bc433217aad` |
| `mapping.witness.json` | `3e12262d5828688b47a25543a87ab6e22bffa2c84b7851b0509d1e3ad61c0e21` |
| `forbidden-controls.json` | `33b106ca985a23e09eb8d3a1b39ca350aac2f3612a41cf70007d18defe20db3f` |
| `boundary.witnesses.json` | `38a258ac70a9d6cf8236b76236e69b707d63394e5ad88a0d74be21c755b51729` |

`TestFixtureBytesPinned` checks these digests, so an edit here is detected.
Do not edit the files; refresh them from a reviewed upstream commit and update
the table, the test pin and the commit above together. The specimen is
shape-positive only: it grants no execution, qualification or Result
authority. The closed control decoder admits exactly 88 controls and resolves
neighbors against all 59 globally unique boundary case ids. Adapter-owned rows
have distinct named tests; other rows retain their owning-component reason.
Fault-injected unit and process tests are not native deployment qualification.
