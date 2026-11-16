# m0adapter fixtures

These four JSON files are verbatim copies of the inert documentary fixtures of
the DRAFT `swarma-dispatcher m0-executor-entry/1` proposal:

- source repository: `github.com/relux-works/swarma-dispatcher`
- source commit: `cf72502` (swarma-dispatcher PR #25, the no-feeder amendment on top of `962eeb9`)
- source directory: `fixtures/m0-executor-entry/`

| File | SHA-256 |
| --- | --- |
| `entry.specimen.json` | `16b898164b1b4bd0568a37bf71cbdf2625dc89f45544730866793bc433217aad` |
| `mapping.witness.json` | `3e12262d5828688b47a25543a87ab6e22bffa2c84b7851b0509d1e3ad61c0e21` |
| `forbidden-controls.json` | `71cd6e8becaac27674dd65f3dc92e7a8e88093e6051e86ce47badae12da7e262` |
| `boundary.witnesses.json` | `a3df6f8ec7cc983bf82cd912a4e881dff308b1458ac75519c362195d37aa3160` |

`TestFixtureBytesPinned` checks these digests, so an edit here is detected.
Do not edit the files; refresh them from a reviewed upstream commit and update
the table, the test pin and the commit above together. The specimen is
shape-positive only: it grants no execution, qualification or Result
authority.
