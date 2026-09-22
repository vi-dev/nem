# Changelog

## [0.10.0](https://github.com/vi-dev/nem/compare/v0.9.0...v0.10.0) (2026-09-22)


### Features

* Add version template helpers and discovery metadata ([7252ac4](https://github.com/vi-dev/nem/commit/7252ac443317dbf03ff2273868223e31e6ab9e17))
* Build packages in dependency-ordered batches ([8bcb105](https://github.com/vi-dev/nem/commit/8bcb10552f2d532c9fffafc5a27498e4a4c3d10c))
* Bump manifests that declare no versions yet ([9ca8f0d](https://github.com/vi-dev/nem/commit/9ca8f0dec7870b22f8b52d04e40a12eafa318440))
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

* Extract zips with prepended data ([b193421](https://github.com/vi-dev/nem/commit/b19342182351cfc104b3572bfd454e95e4d85151))
* Ignore dot components when stripping archive paths ([8db2372](https://github.com/vi-dev/nem/commit/8db23726e80b5b4fc3b8e762dd9b60fb660e0999))
* Parse config and install metadata YAML leniently ([dddc637](https://github.com/vi-dev/nem/commit/dddc6378628da55a59842621c97c5b754fd41fce))


### Refactors

* Implement catalog set and manifest editor ([b9c8850](https://github.com/vi-dev/nem/commit/b9c8850aca6d4140fcea7d0ef675b4453c49f3ed))
* Merge Task.Progress and Count into unit-aware Progress ([08675b0](https://github.com/vi-dev/nem/commit/08675b02af76265f6916cf6cde0c0592620e78b2))
* Pass reporter via context instead of explicit args ([31f32da](https://github.com/vi-dev/nem/commit/31f32da1b3b3f4ce5f6b145d602a5ea2ec310027))
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
