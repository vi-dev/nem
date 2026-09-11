# Changelog

## [0.10.0](https://github.com/vi-dev/nem/compare/v0.9.0...v0.10.0) (2026-09-11)


### Features

* Add version template helpers and discovery metadata ([7252ac4](https://github.com/vi-dev/nem/commit/7252ac443317dbf03ff2273868223e31e6ab9e17))
* Bump manifests that declare no versions yet ([9ca8f0d](https://github.com/vi-dev/nem/commit/9ca8f0dec7870b22f8b52d04e40a12eafa318440))
* Resolve catalog test manifests without configuration ([862e2e9](https://github.com/vi-dev/nem/commit/862e2e992e7a3483b621316b91b3873206e93f90))
* Support hardlinks in tar archives ([80e7211](https://github.com/vi-dev/nem/commit/80e7211fa57b8a3ff8378ea7a85bccda677e3d5a))


### Bug Fixes

* Extract zips with prepended data ([b193421](https://github.com/vi-dev/nem/commit/b19342182351cfc104b3572bfd454e95e4d85151))
* Ignore dot components when stripping archive paths ([8db2372](https://github.com/vi-dev/nem/commit/8db23726e80b5b4fc3b8e762dd9b60fb660e0999))
* Parse config and install metadata YAML leniently ([dddc637](https://github.com/vi-dev/nem/commit/dddc6378628da55a59842621c97c5b754fd41fce))


### Refactors

* Merge Task.Progress and Count into unit-aware Progress ([08675b0](https://github.com/vi-dev/nem/commit/08675b02af76265f6916cf6cde0c0592620e78b2))

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
