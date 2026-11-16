# m0adapter fixtures

These four JSON files are verbatim copies of the inert documentary fixtures of
the DRAFT `swarma-dispatcher m0-executor-entry/1` proposal:

- source repository: `github.com/relux-works/swarma-dispatcher`
- source commit: `0772cf3` (swarma-dispatcher PR #25 after its rebase: the no-feeder amendment and the Claude runner variant; the fixture bytes equal those at the earlier `c19c58b`)
- source directory: `fixtures/m0-executor-entry/`

| File | SHA-256 |
| --- | --- |
| `entry.specimen.json` | `16b898164b1b4bd0568a37bf71cbdf2625dc89f45544730866793bc433217aad` |
| `mapping.witness.json` | `3e12262d5828688b47a25543a87ab6e22bffa2c84b7851b0509d1e3ad61c0e21` |
| `forbidden-controls.json` | `e1f75a1921b3f2f625e5e53ae302c22aa011c4e301f70387b83ec8099a2944e5` |
| `boundary.witnesses.json` | `a3df6f8ec7cc983bf82cd912a4e881dff308b1458ac75519c362195d37aa3160` |

`TestFixtureBytesPinned` checks these digests, so an edit here is detected.
Do not edit the files; refresh them from a reviewed upstream commit and update
the table, the test pin and the commit above together. The specimen is
shape-positive only: it grants no execution, qualification or Result
authority.
