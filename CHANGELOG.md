# Changelog

## [2.1.1](https://github.com/dknathalage/tallyo/compare/v2.1.0...v2.1.1) (2026-05-02)


### Bug Fixes

* resolve drizzle migrations folder from package root at runtime ([6f73daa](https://github.com/dknathalage/tallyo/commit/6f73daabfef54743de35f4f6378210d5164d6c8b))

## [2.1.0](https://github.com/dknathalage/tallyo/compare/v2.0.0...v2.1.0) (2026-05-02)


### Features

* rebrand Invoice Manager to Tallyo ([cecf9f8](https://github.com/dknathalage/tallyo/commit/cecf9f8ea284e46ca315700cdc2a03baf8d8b668))

## [2.0.0](https://github.com/dknathalage/tallyo/compare/v1.0.2...v2.0.0) (2026-05-01)


### ⚠ BREAKING CHANGES

* requires PostgreSQL instead of SQLite, set DATABASE_URL env var

### Features

* add Docker support and deployment improvements ([#85](https://github.com/dknathalage/tallyo/issues/85)) ([b9a94df](https://github.com/dknathalage/tallyo/commit/b9a94df8581bf866f261d11f0a6c0718587cb110))
* add keyboard shortcuts and quick-add FAB ([#88](https://github.com/dknathalage/tallyo/issues/88)) ([2f30c9c](https://github.com/dknathalage/tallyo/commit/2f30c9c3eb9ff5ae0d931c9654bf163abb9c0cff))
* add one-command curl installer ([43f6765](https://github.com/dknathalage/tallyo/commit/43f6765b56fd26e2222017c51ca33191b2213477))
* add pagination to list endpoints ([#90](https://github.com/dknathalage/tallyo/issues/90)) ([5034240](https://github.com/dknathalage/tallyo/commit/5034240790b8fdd26e9e7f6a7754c1f4a5d9edde))
* add tallyo CLI entry ([21a7cd2](https://github.com/dknathalage/tallyo/commit/21a7cd2a8dde880dfd9841fb2cc8ed81f5cd6ccf))
* add version display in sidebar and self-updating infrastructure ([be370e9](https://github.com/dknathalage/tallyo/commit/be370e90d12cfc2111051610457231a6eeba27af))
* add Zod validation to all API routes ([#87](https://github.com/dknathalage/tallyo/issues/87)) ([f2ad37f](https://github.com/dknathalage/tallyo/commit/f2ad37fd36d35a74cd9c5efe426262f977a6284d))
* AI chat assistant backend - DB, tools, streaming SSE API ([#92](https://github.com/dknathalage/tallyo/issues/92)) ([e3327b9](https://github.com/dknathalage/tallyo/commit/e3327b9a634f8da51bbc9497840ccc39f736dbd4))
* AI chat assistant frontend ([#91](https://github.com/dknathalage/tallyo/issues/91)) ([088d82f](https://github.com/dknathalage/tallyo/commit/088d82f4333fb15a4a384f321dd5f79f5ddb5e3c))
* automatically mark overdue invoices on app load ([#37](https://github.com/dknathalage/tallyo/issues/37)) ([6374bd1](https://github.com/dknathalage/tallyo/commit/6374bd1952dea1fab3f78a6447ac2f61048a9b59))
* database backup and restore ([#24](https://github.com/dknathalage/tallyo/issues/24)) ([#36](https://github.com/dknathalage/tallyo/issues/36)) ([b55294b](https://github.com/dknathalage/tallyo/commit/b55294b8edb6ab39e842ae8f36e92752aa839679))
* define repository interfaces for all data entities ([#16](https://github.com/dknathalage/tallyo/issues/16)) ([#21](https://github.com/dknathalage/tallyo/issues/21)) ([9d9ee60](https://github.com/dknathalage/tallyo/commit/9d9ee60652216d0a45fc1d2662362f192b4eab3a))
* derive DB path dynamically from package.json name ([226bee2](https://github.com/dknathalage/tallyo/commit/226bee295121dd02480a6cb08a305dc02992b461))
* duplicate invoice and estimate ([#40](https://github.com/dknathalage/tallyo/issues/40)) ([a9f0433](https://github.com/dknathalage/tallyo/commit/a9f04337249a12bb6551f306b0b230ab776f5e14))
* embed git short sha in version, run migrations at install ([0f0e41b](https://github.com/dknathalage/tallyo/commit/0f0e41b05026615e6dfa2967d64819fcb90138e5))
* implement SqliteRepository per entity and wire up via registry ([#17](https://github.com/dknathalage/tallyo/issues/17)) ([#22](https://github.com/dknathalage/tallyo/issues/22)) ([b5db59a](https://github.com/dknathalage/tallyo/commit/b5db59ab96be295db89a5065676f7dce421844f0))
* invoice aging report ([#45](https://github.com/dknathalage/tallyo/issues/45)) ([fddef2e](https://github.com/dknathalage/tallyo/commit/fddef2e2264c07383da8d7dd546e0073190b06b2)), closes [#33](https://github.com/dknathalage/tallyo/issues/33)
* keyboard shortcuts for common actions ([#43](https://github.com/dknathalage/tallyo/issues/43)) ([92886ef](https://github.com/dknathalage/tallyo/commit/92886ef583e2bb7a1e51832d51ca6472430d9590))
* migrate database from PostgreSQL to SQLite with better-sqlite3 ([b2a56b4](https://github.com/dknathalage/tallyo/commit/b2a56b4865553294fbf473886c2fe7a8c90b5978))
* migrate database from SQLite to PostgreSQL with Drizzle ORM ([3996cb4](https://github.com/dknathalage/tallyo/commit/3996cb4e651484325d4cd40360fe5b8f8c698275))
* monthly revenue trend chart on dashboard ([#39](https://github.com/dknathalage/tallyo/issues/39)) ([57e6e0a](https://github.com/dknathalage/tallyo/commit/57e6e0ac1effa1d7a2c972e58c4cb6f876f3e24b)), closes [#25](https://github.com/dknathalage/tallyo/issues/25)
* multiple named tax rates ([#44](https://github.com/dknathalage/tallyo/issues/44)) ([a0ddc74](https://github.com/dknathalage/tallyo/commit/a0ddc74fed539b51f9ae3c5762d49fd12c200f3d))
* payment recording with partial payment support ([#46](https://github.com/dknathalage/tallyo/issues/46)) ([ba5f1f9](https://github.com/dknathalage/tallyo/commit/ba5f1f9dac0dab50e58b5cfb117f86ec56e9b21e))
* payment terms presets on invoice form ([#38](https://github.com/dknathalage/tallyo/issues/38)) ([78346ea](https://github.com/dknathalage/tallyo/commit/78346ea7bd33030e6e87df8b906c21024dbd924b))
* per-client revenue summary on client detail page ([#42](https://github.com/dknathalage/tallyo/issues/42)) ([efd1f84](https://github.com/dknathalage/tallyo/commit/efd1f84e4916fd3d10cd609769bbed07fd9f53b7)), closes [#28](https://github.com/dknathalage/tallyo/issues/28)
* recurring invoice templates ([#26](https://github.com/dknathalage/tallyo/issues/26)) ([#47](https://github.com/dknathalage/tallyo/issues/47)) ([65f430b](https://github.com/dknathalage/tallyo/commit/65f430bd9dff337125f9aea33009e6ca4f7c8511))
* server-side SQLite with better-sqlite3 — fixes type errors, adapter-node ([#57](https://github.com/dknathalage/tallyo/issues/57)) ([2010a4a](https://github.com/dknathalage/tallyo/commit/2010a4a7c0d40f905a0f2679730049998925ea07))
* share invoice via mailto link ([#41](https://github.com/dknathalage/tallyo/issues/41)) ([a657777](https://github.com/dknathalage/tallyo/commit/a657777d0fb9628192263e2cdbcebdd6ca1c63b6))
* StorageTransaction + abstract audit logging ([#18](https://github.com/dknathalage/tallyo/issues/18)) ([#23](https://github.com/dknathalage/tallyo/issues/23)) ([03a9f9a](https://github.com/dknathalage/tallyo/commit/03a9f9a760cb03164c6775e678530b3926ec4796))
* toast notification system for consistent error feedback ([#51](https://github.com/dknathalage/tallyo/issues/51)) ([e543197](https://github.com/dknathalage/tallyo/commit/e543197b1034b1d964e3d8c9fa8b1f6e78fdb985))


### Bug Fixes

* add native build tools to Dockerfile and pin better-sqlite3 version ([9b87108](https://github.com/dknathalage/tallyo/commit/9b8710851bbb5cd12331d57875bbe6a2dff2e6b5))
* align installer banner box ([205bb90](https://github.com/dknathalage/tallyo/commit/205bb905eba74ff8703a740a2be5a297e3744cf5))
* all 224 tests passing + coverage setup ([#59](https://github.com/dknathalage/tallyo/issues/59)) ([c37c313](https://github.com/dknathalage/tallyo/commit/c37c31323699b85a68d41cbc263ef0a10a7ec34a))
* catalog import failing with 500 due to body size limit ([7d7bbb3](https://github.com/dknathalage/tallyo/commit/7d7bbb3f3ef4060083b768d3ed37fa7483421b45))
* catalog import skips invalid/unknown tier IDs ([#72](https://github.com/dknathalage/tallyo/issues/72)) ([dc73d0a](https://github.com/dknathalage/tallyo/commit/dc73d0adc62d80d48cb624e3b032a735fa1b45b4)), closes [#59](https://github.com/dknathalage/tallyo/issues/59)
* generateInvoiceNumber() no longer produces INV-NaN or collides ([#14](https://github.com/dknathalage/tallyo/issues/14)) ([143cd49](https://github.com/dknathalage/tallyo/commit/143cd49a2f916013ea1bba4c97273133ecaed21c)), closes [#1](https://github.com/dknathalage/tallyo/issues/1)
* handle paginated catalog response, tier conflicts, sync transactions, and i18n keys ([7e6b7cb](https://github.com/dknathalage/tallyo/commit/7e6b7cb8c5aecc8e83ed7a4a30dd774f2b0253b7))
* improve code quality and robustness ([#89](https://github.com/dknathalage/tallyo/issues/89)) ([8daba79](https://github.com/dknathalage/tallyo/commit/8daba79167ab8af19667c3f6d4459da13022173e))
* move DB reads out of $derived, replace importTrigger counter pattern ([#52](https://github.com/dknathalage/tallyo/issues/52)) ([2fcda2e](https://github.com/dknathalage/tallyo/commit/2fcda2e9630e850e0cabe1895589d50a72712797))
* move invoice/estimate number generation to server-side ([#69](https://github.com/dknathalage/tallyo/issues/69)) ([588b76b](https://github.com/dknathalage/tallyo/commit/588b76b20a2d972d4ec41b62f41333406912ea7c))
* proper 409/400 errors for constraint violations ([#71](https://github.com/dknathalage/tallyo/issues/71)) ([a6cccea](https://github.com/dknathalage/tallyo/commit/a6cccea16b89f46be676558f151b615912922032)), closes [#59](https://github.com/dknathalage/tallyo/issues/59)
* PWA manifest 404 + $state not defined in toast store ([#58](https://github.com/dknathalage/tallyo/issues/58)) ([cf41f82](https://github.com/dknathalage/tallyo/commit/cf41f8287b594db0310bba5e2020c03c2d3ef1c9))
* redirect favicon.ico to favicon.svg ([#73](https://github.com/dknathalage/tallyo/issues/73)) ([1acd3ee](https://github.com/dknathalage/tallyo/commit/1acd3ee44945d0c34492491fd2aeec0bc0ef3463))
* remove duplicate GPL LICENCE file ([#80](https://github.com/dknathalage/tallyo/issues/80)) ([e3c7206](https://github.com/dknathalage/tallyo/commit/e3c7206732b38ca3f8c1282a4a38d090bdb87ca2))
* remove plan from directory ([e9734df](https://github.com/dknathalage/tallyo/commit/e9734df80f11a5658ce22751d4bdf1692cd4d1cc))
* remove ssr.noExternal for better-sqlite3 ([#70](https://github.com/dknathalage/tallyo/issues/70)) ([8dcd6c1](https://github.com/dknathalage/tallyo/commit/8dcd6c16fb1a3ad4edafc62e47dd3644e9e7752b))
* resolve CI type errors and align release workflow with CI steps ([af06a37](https://github.com/dknathalage/tallyo/commit/af06a3790ba6f8299d6535eb2c1c1d81e70c1d51))
* resolve TypeScript type errors and gate auto-tag on CI success ([9a56f31](https://github.com/dknathalage/tallyo/commit/9a56f318adf3e884a41894c31739a51f591fee9c))
* systemic API error handling — proper 400/409 for all constraint violations ([#74](https://github.com/dknathalage/tallyo/issues/74)) ([7480ce4](https://github.com/dknathalage/tallyo/commit/7480ce40eccc6493c3e254180d40da7e2f25d414))
* type safety improvements across Invoice types, logAudit, and CSV types ([#49](https://github.com/dknathalage/tallyo/issues/49)) ([534ee25](https://github.com/dknathalage/tallyo/commit/534ee25fed36a0b19024bf8428ea322708386fe2))
* update Zod v4 API usage from .errors to .issues ([052c0b7](https://github.com/dknathalage/tallyo/commit/052c0b728e60c38c3d0faa2b9c854b2e47a67110))
* use git short SHA as version instead of package.json bump ([44a68f6](https://github.com/dknathalage/tallyo/commit/44a68f6907c62a48a8ed1da245a741908aa566d3))
* use join(..) instead of dirname to avoid browser-external issue ([#68](https://github.com/dknathalage/tallyo/issues/68)) ([f976b55](https://github.com/dknathalage/tallyo/commit/f976b55be030f3800990b70e343d294f58c50d54))
* wrap deleteInvoice/deleteEstimate/deleteClient in transactions ([#15](https://github.com/dknathalage/tallyo/issues/15)) ([01e2c47](https://github.com/dknathalage/tallyo/commit/01e2c47430c1a1a8539c4b423f92760319073c35)), closes [#3](https://github.com/dknathalage/tallyo/issues/3)


### Performance Improvements

* add DB indexes and fix N+1 queries ([#86](https://github.com/dknathalage/tallyo/issues/86)) ([a26f4dc](https://github.com/dknathalage/tallyo/commit/a26f4dc802bf07cbe974cf18417b955dcd9ecc04))
