# m0adapter fixtures

These four JSON files are verbatim copies of the inert documentary fixtures of
the DRAFT `swarma-dispatcher m0-executor-entry/1` proposal:

- source repository: `github.com/relux-works/swarma-dispatcher`
- source commit: `962eeb9`
- source directory: `fixtures/m0-executor-entry/`

| File | SHA-256 |
| --- | --- |
| `entry.specimen.json` | `16b898164b1b4bd0568a37bf71cbdf2625dc89f45544730866793bc433217aad` |
| `mapping.witness.json` | `3e12262d5828688b47a25543a87ab6e22bffa2c84b7851b0509d1e3ad61c0e21` |
| `forbidden-controls.json` | `6ba57c7beb6a52e2621cee6c9706c09df27b694ee310c9796d27aa433ed5f07b` |
| `boundary.witnesses.json` | `3f5d29ff8df7ea758db33d94e96d95504ebdc8ff3f795d02ea75ea1982745ffa` |

`TestFixtureBytesPinned` checks these digests, so an edit here is detected.
Do not edit the files; refresh them from a reviewed upstream commit and update
the table, the test pin and the commit above together. The specimen is
shape-positive only: it grants no execution, qualification or Result
authority.
