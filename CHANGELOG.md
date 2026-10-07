# Changelog

## [0.10.1](https://github.com/vi-dev/nem/compare/v0.10.0...v0.10.1) (2026-10-07)


### Bug Fixes

* Report an unrunnable command instead of exiting silently ([19e57ea](https://github.com/vi-dev/nem/commit/19e57ea496dddd95272f3e21b067cb697f0d5743))

## [0.10.0](https://github.com/vi-dev/nem/compare/v0.9.0...v0.10.0) (2026-10-01)


### Features

* Add hidden gendocs command and make docs target ([af67971](https://github.com/vi-dev/nem/commit/af67971489da3c1b7f5a939381877706b31b09a5))
* Add version template helpers and discovery metadata ([7252ac4](https://github.com/vi-dev/nem/commit/7252ac443317dbf03ff2273868223e31e6ab9e17))
* Animate live task lines with a spinner ([d5b4431](https://github.com/vi-dev/nem/commit/d5b44310bbd4510e86cf497d823e4b1bae673c8a))
* Build packages in dependency-ordered batches ([8bcb105](https://github.com/vi-dev/nem/commit/8bcb10552f2d532c9fffafc5a27498e4a4c3d10c))
* Bump manifests that declare no versions yet ([9ca8f0d](https://github.com/vi-dev/nem/commit/9ca8f0dec7870b22f8b52d04e40a12eafa318440))
* Command `catalog build` narrates each build as a live task ([6d1851a](https://github.com/vi-dev/nem/commit/6d1851a770feba69c1768629e5655162e4c537e3))
* Link dependencies through `.nem-link-dependencies` symlinks ([5303f64](https://github.com/vi-dev/nem/commit/5303f64c678d79b2519e17cea9f4652ac33f2217))
* Prefix debug, prompt lines and echo answers inline ([e57064a](https://github.com/vi-dev/nem/commit/e57064a33902f1419192ac264b4901cfdd069055))
* Prefix Info output lines with a glyph ([9d5f0f9](https://github.com/vi-dev/nem/commit/9d5f0f931369c682de55f0cff3d6e65f9a72a17a))
* Render a command tree as Markdown reference pages ([cc18d07](https://github.com/vi-dev/nem/commit/cc18d07707c1e7b4b33c8f9ef9c4214f418c88d0))
* Resolve catalog test manifests without configuration ([862e2e9](https://github.com/vi-dev/nem/commit/862e2e992e7a3483b621316b91b3873206e93f90))
* Scope catalog lint/fmt/test with a catalog arg and --package flag ([a64dc4c](https://github.com/vi-dev/nem/commit/a64dc4c9ad7e78ca48c68d88725b21dccb225bb4))
* Support hardlinks in tar archives ([80e7211](https://github.com/vi-dev/nem/commit/80e7211fa57b8a3ff8378ea7a85bccda677e3d5a))
* Update `catalog build` command contract ([eae476c](https://github.com/vi-dev/nem/commit/eae476c0fb0e19e2cde8995e72f6dbd8b7a624b3))
* Update `catalog bump` command contract ([b4418b6](https://github.com/vi-dev/nem/commit/b4418b629bd89f9a0b79a7bd647341f8678950d5))
* Update `catalog diff` command contract ([1247b1b](https://github.com/vi-dev/nem/commit/1247b1bc39517a098c9c1b1ec831a5512b629687))
* Update `catalog fill` command contract ([5b08052](https://github.com/vi-dev/nem/commit/5b08052009d0d92ed8391f5c25061e1cbc6cc71f))
* Update `catalog mirror` command contract ([508b1f5](https://github.com/vi-dev/nem/commit/508b1f5068fbde961b9eb671f663ec5077e09f36))
* Update `catalog outdated` command contract ([1eaa0c5](https://github.com/vi-dev/nem/commit/1eaa0c5bcfbe5af8f3cc5fe4149869252d0a8b35))
* Update `catalog publish` command contract ([eb42c30](https://github.com/vi-dev/nem/commit/eb42c30e3dff6ec6c55af9fe87d31ddaa8d30bba))
* Update `catalog test` command contract ([871970b](https://github.com/vi-dev/nem/commit/871970b5620ef6722602ac517fdfa0c517721a9c))


### Bug Fixes

* Catch braced $ORIGIN and interior climbs in rpath checks ([512f97c](https://github.com/vi-dev/nem/commit/512f97c6d3afff661de8547b49d3040f8c87a19e))
* Command `catalog bump` must honor context cancellation ([3accd75](https://github.com/vi-dev/nem/commit/3accd75edadf3b73d9cfd5f0bb9893e5c5ff6649))
* **deps:** Update go dependencies (non-major) ([#8](https://github.com/vi-dev/nem/issues/8)) ([767da34](https://github.com/vi-dev/nem/commit/767da34f66a33ee1622610a4fcddf09e5cea875f))
* Extract zips with prepended data ([b193421](https://github.com/vi-dev/nem/commit/b19342182351cfc104b3572bfd454e95e4d85151))
* Ignore dot components when stripping archive paths ([8db2372](https://github.com/vi-dev/nem/commit/8db23726e80b5b4fc3b8e762dd9b60fb660e0999))
* Name the catalog archive store in the relative oci ref error ([b49ba4a](https://github.com/vi-dev/nem/commit/b49ba4a4d0a03eb62fe583c840f6f8e0df1ee169))
* Parse config and install metadata YAML leniently ([dddc637](https://github.com/vi-dev/nem/commit/dddc6378628da55a59842621c97c5b754fd41fce))
* Re-apply the environment after nem update ([a85e9db](https://github.com/vi-dev/nem/commit/a85e9dbc8bea22b3615982d869f15fc5d0851ba3))
* Relink dependencies only after every install succeeds ([009db35](https://github.com/vi-dev/nem/commit/009db3583ab7cfdbeda89dc37d20bd4210c99ef1))
* Sync a never-synced catalog store before installing ([7c8b0f5](https://github.com/vi-dev/nem/commit/7c8b0f5724d95773dd26b30b62845e57a7bf3113))
* Unuse command installs and relinks after re-resolving the lock ([d44e676](https://github.com/vi-dev/nem/commit/d44e6760b412b664a26aec51441b2e30180a8248))
* Warn when a package's dependency links cannot be read ([2156d5d](https://github.com/vi-dev/nem/commit/2156d5d855ab4b5a4a8a1554d94606c90416073d))


### Refactors

* Clearer output and hints for `catalog bump` command ([3e446e0](https://github.com/vi-dev/nem/commit/3e446e06bdb3b20970289277ddcdc59cbeb30c20))
* Implement catalog set and manifest editor ([b9c8850](https://github.com/vi-dev/nem/commit/b9c8850aca6d4140fcea7d0ef675b4453c49f3ed))
* Merge Task.Progress and Count into unit-aware Progress ([08675b0](https://github.com/vi-dev/nem/commit/08675b02af76265f6916cf6cde0c0592620e78b2))
* Pass reporter via context instead of explicit args ([31f32da](https://github.com/vi-dev/nem/commit/31f32da1b3b3f4ce5f6b145d602a5ea2ec310027))
* Replace --json with --output for `bump` and `outdated` commands ([cc93d8b](https://github.com/vi-dev/nem/commit/cc93d8b9239a7935adc166d3bbd1c349005a46f4))
* Resolve catalog locations through catalog.Open ([f7d4b77](https://github.com/vi-dev/nem/commit/f7d4b77d35bd23847dd39e677285235c7576d734))
* Resolve lint/fmt catalog through catalog.Open ([7568628](https://github.com/vi-dev/nem/commit/756862802b4fb1dc236b82cb4b03f5d0c8fa740a))
* Thread ctx through catalog.Editor methods ([9fcece1](https://github.com/vi-dev/nem/commit/9fcece1b2ba67f3d13dde0f31b7bb573c5ba1d4c))
* Unify archive storage behind internal/archive ([532f07a](https://github.com/vi-dev/nem/commit/532f07a2dc9f9472a6dfa8e3eeeeaad15e90591c))

## [0.9.0](https://github.com/vi-dev/nem/compare/v0.0.0...v0.9.0) (2026-09-09)


### Features

* Add `catalog lint` and `publish` ([25a83d2](https://github.com/vi-dev/nem/commit/25a83d24bca5f9ff51784dc011a718ae20276f00))
* Add `nem catalog build` (local source-build engine) and --push ([25a83d2](https://github.com/vi-dev/nem/commit/25a83d24bca5f9ff51784dc011a718ae20276f00))
* Add autotools and generator tools to the build image ([25a83d2](https://github.com/vi-dev/nem/commit/25a83d24bca5f9ff51784dc011a718ae20276f00))
* Add foundations ([25a83d2](https://github.com/vi-dev/nem/commit/25a83d24bca5f9ff51784dc011a718ae20276f00))
* Add installation script ([086392f](https://github.com/vi-dev/nem/commit/086392fe1159a9ba70cca2628d550293cc30fae8))
* Add rootless runtime image variant ([25a83d2](https://github.com/vi-dev/nem/commit/25a83d24bca5f9ff51784dc011a718ae20276f00))
* Add short aliases for common commands ([25a83d2](https://github.com/vi-dev/nem/commit/25a83d24bca5f9ff51784dc011a718ae20276f00))
* Group commands in help output ([25a83d2](https://github.com/vi-dev/nem/commit/25a83d24bca5f9ff51784dc011a718ae20276f00))
* Implement `nem` CLI ([25a83d2](https://github.com/vi-dev/nem/commit/25a83d24bca5f9ff51784dc011a718ae20276f00))
* Make resolution order-independent via two-phase selection ([25a83d2](https://github.com/vi-dev/nem/commit/25a83d24bca5f9ff51784dc011a718ae20276f00))
* Move official catalog to its rightfully location ([7a5f0b3](https://github.com/vi-dev/nem/commit/7a5f0b32fb7be37a607fbb24e1daf9182e83a5cd))
* Shell completions ([25a83d2](https://github.com/vi-dev/nem/commit/25a83d24bca5f9ff51784dc011a718ae20276f00))


### Chores

* **main:** release 0.9.0 ([5e8178d](https://github.com/vi-dev/nem/commit/5e8178dcb0b591e5f8f0be2b3089ecbefc702ebd))
