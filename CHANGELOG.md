# Changelog

## [3.5.0](https://github.com/dknathalage/tallyo/compare/v3.4.0...v3.5.0) (2026-06-05)


### Features

* **app:** wire DB+migrate on startup and bind BusinessProfileService ([20387fd](https://github.com/dknathalage/tallyo/commit/20387fd50c7311bef8ba1f4f68fd2ad4d866b096))
* **audit:** Log helper writing audit_log rows for mutations ([85250a3](https://github.com/dknathalage/tallyo/commit/85250a3f75cb0f12bd32a198184447c9c5451cc0))
* **audit:** WithTx audited-mutation wrapper and Changes helper ([cc1aae3](https://github.com/dknathalage/tallyo/commit/cc1aae33e72cb4cc55ce3735e657e92ad3055f0c))
* **auth:** bcrypt password hashing and verification ([2305358](https://github.com/dknathalage/tallyo/commit/2305358404bc2553dbc3901b605ea56caa3c3b5c))
* **auth:** invite repository with secure token, expiry, and audited lifecycle ([95aee14](https://github.com/dknathalage/tallyo/commit/95aee14604dbde54d3aee2936131bdcc39226b2c))
* **auth:** scs session manager backed by sqlite store (+ integration test) ([fd54473](https://github.com/dknathalage/tallyo/commit/fd54473d3444d203d5684ed9fda823b9dae17d06))
* **auth:** user repository with audited create/delete and credentials lookup ([72d73d7](https://github.com/dknathalage/tallyo/commit/72d73d7eb6cbf47293e88b57d455437a3211b740))
* **cmd:** wire rate tier + payer services ([40624f8](https://github.com/dknathalage/tallyo/commit/40624f87dc57bf6adffd05996c8778327113f0f5))
* **cmd:** wire serve command end-to-end with graceful shutdown ([e416d31](https://github.com/dknathalage/tallyo/commit/e416d31a40aa965092a164294ebbc7310e54a5aa))
* **cmd:** wire tax rate + client + catalog services ([8f1c864](https://github.com/dknathalage/tallyo/commit/8f1c8646046fdcf953ca9f457946d43fda9193c7))
* **db:** auth schema (users, invites, scs sessions) + sqlc queries ([6a835c8](https://github.com/dknathalage/tallyo/commit/6a835c8af59a03d854a352a7b4652348dd367937))
* **db:** DSN pragmas + connection pool for concurrent web-server access ([7ea5ae3](https://github.com/dknathalage/tallyo/commit/7ea5ae3f78ca61e4e753d4fff6c13de6848dbd8d))
* **db:** embedded goose migrations for audit_log and business_profile ([513f06c](https://github.com/dknathalage/tallyo/commit/513f06ce8ca092c797ec64bfa4f26a688fead175))
* **db:** estimates + estimate_line_items migration and sqlc queries ([211f024](https://github.com/dknathalage/tallyo/commit/211f0246a6130e52119d7c550b16d3d703c07faf))
* **db:** invoices + line_items migration and sqlc queries ([1fd285c](https://github.com/dknathalage/tallyo/commit/1fd285cb5eeb4f805301107792a75097f47f1ab7))
* **db:** modernc sqlite connection with WAL pragmas and data dir ([2be864c](https://github.com/dknathalage/tallyo/commit/2be864cb24949046c563ff1bad9a5a89807166de))
* **db:** payments migration, queries, and client total_paid stats ([07cae54](https://github.com/dknathalage/tallyo/commit/07cae5448aacc653b0557d20fc8263d9b3d4cba3))
* **db:** rate_tiers + payers migration and sqlc queries ([e1494d4](https://github.com/dknathalage/tallyo/commit/e1494d4a344a5b994c24d5488978aacf1ad0fc05))
* **db:** recurring_templates migration and sqlc queries ([a7ab7b9](https://github.com/dknathalage/tallyo/commit/a7ab7b994401ab24300a058f1ed2c32d8ddfb7c2))
* **db:** sqlc business_profile queries verified against modernc driver ([69c0c8f](https://github.com/dknathalage/tallyo/commit/69c0c8f20d29f3cdf3c46b3687cc49aee0fb4ad0))
* **db:** tax_rates + clients + catalog migration and sqlc queries ([d12f661](https://github.com/dknathalage/tallyo/commit/d12f6619b50840c2d08621d1395afa815ecca2da))
* embed SvelteKit SPA into binary; full web-service skeleton works end to end ([28f9174](https://github.com/dknathalage/tallyo/commit/28f9174fd67a0e2eac238dd51d4f7d410b7052b6))
* **estimates:** service, REST endpoints, and convert-to-invoice ([592b8a5](https://github.com/dknathalage/tallyo/commit/592b8a5437ef657a11551f25b124294bca9de222))
* **export:** CSV + Excel export for catalog, invoices, estimates ([418f8ab](https://github.com/dknathalage/tallyo/commit/418f8ab913cda79ef4772368c90e32159413f71d))
* **frontend:** settings view loads and saves business profile via binding ([bec9485](https://github.com/dknathalage/tallyo/commit/bec948545bc2ac697f4ff9f1b947703a1738753d))
* **http:** business profile GET/PUT endpoints behind auth ([ab222ee](https://github.com/dknathalage/tallyo/commit/ab222ee8b3b218263cf77465d6e540cbf73e6aad))
* **http:** first-run setup with owner creation and 409 guard ([8c46441](https://github.com/dknathalage/tallyo/commit/8c46441a2913c05294200ed277e24857f10f754b))
* **http:** invite creation (owner-only), validation, and acceptance ([9d51946](https://github.com/dknathalage/tallyo/commit/9d51946a1215cd1940604194f61c28f185e74be0))
* **http:** invoice + estimate PDF download endpoints ([6178f51](https://github.com/dknathalage/tallyo/commit/6178f51d13e7d31593f85874333c9f593a9243b3))
* **http:** rate tier + payer REST endpoints ([59c3398](https://github.com/dknathalage/tallyo/commit/59c3398bee9e8e1f7d7f97cb18abd1c2cf87c98f))
* **http:** server scaffold with JSON helpers and SPA static handler ([b6417fd](https://github.com/dknathalage/tallyo/commit/b6417fdfe93ea65093d21c06515d147b5abb4280))
* **http:** session login/logout/me with auth-guard and user-exists recheck ([1a8ef6e](https://github.com/dknathalage/tallyo/commit/1a8ef6e319684298938fc9225113dbab2d47e31d))
* **http:** SSE /api/events endpoint streaming change events ([f04bc1e](https://github.com/dknathalage/tallyo/commit/f04bc1efc2dc570edbfc969090c924d38171b24b))
* **http:** tax rate + client + catalog REST endpoints ([94d51c1](https://github.com/dknathalage/tallyo/commit/94d51c1aa92582e59a305bdeed5da35250bdcb70))
* **import:** catalog import with parse, map, diff, and commit ([504f988](https://github.com/dknathalage/tallyo/commit/504f9885c800b8eab748df054666e161684720b9))
* **import:** column_mappings CRUD full-stack backend ([9e4f800](https://github.com/dknathalage/tallyo/commit/9e4f800cc18e3e3b8acd1ae7a0c5159fbac35742))
* **invoices:** service, REST endpoints, and overdue sweep ([ca684bc](https://github.com/dknathalage/tallyo/commit/ca684bc92d81b3e51523fa7c07fe16dbbfe0747f))
* **numbering:** tx-scoped document numbers with retry-on-conflict ([dcc1401](https://github.com/dknathalage/tallyo/commit/dcc1401e0a30f3cf8167185fa23af28c6392497c))
* **payments:** service, REST endpoints, invoice balance ([490b467](https://github.com/dknathalage/tallyo/commit/490b46728ab6ab2c88aff59a8749fae61ceeab50))
* **pdf:** maroto invoice + estimate PDF rendering ([d86ea8b](https://github.com/dknathalage/tallyo/commit/d86ea8b26e1a3ac5083b0d3d3e060e5c41a0e343))
* **realtime:** in-process SSE hub with bounded per-client buffers ([1717526](https://github.com/dknathalage/tallyo/commit/1717526ded4d14a0016e68a37373dce7fb199d42))
* **recurring:** service, REST endpoints, scheduled generation ([7d7cef7](https://github.com/dknathalage/tallyo/commit/7d7cef71c81db64fbefc2b8c0b5d4aba194a3d57))
* **repository:** business profile repo over sqlc gen with audited upsert ([efac286](https://github.com/dknathalage/tallyo/commit/efac2860137a09ccaf05f55f3ba3ae167fa8f582))
* **repository:** catalog repository with per-tier rates ([47c3257](https://github.com/dknathalage/tallyo/commit/47c3257bd54658cd0478b53017ff2a9b7ab01b75))
* **repository:** client repository with tier/payer joins and bulk delete ([b257c46](https://github.com/dknathalage/tallyo/commit/b257c462621d6c48c1ecb834c2c86763a943820d))
* **repository:** estimate repository with convert-to-invoice ([cbecb8f](https://github.com/dknathalage/tallyo/commit/cbecb8fa7342dfb7d5fff4028ad9a0a7cea1cb9c))
* **repository:** invoice repository with numbering, snapshots, totals, line items ([15c137d](https://github.com/dknathalage/tallyo/commit/15c137dfa746f01be62c2772a7657c0c50fc6d29))
* **repository:** payer repository with search and bulk delete ([95d7cd2](https://github.com/dknathalage/tallyo/commit/95d7cd2b0f69d62830de5bf0ba1bf1cf9fac7f49))
* **repository:** payment repository and invoice paid/balance rollup ([3e2ec9c](https://github.com/dknathalage/tallyo/commit/3e2ec9ce18df83aded653327b77044aba6c5a0fb))
* **repository:** rate tier repository with last-tier delete guard ([3553b87](https://github.com/dknathalage/tallyo/commit/3553b87c46d90cf341ba0c4ee3ed084d2ab5fa3b))
* **repository:** recurring templates with idempotent invoice generation ([d4f490d](https://github.com/dknathalage/tallyo/commit/d4f490d159d03a1a52a3ac692e59246bc224ca1e))
* **repository:** tax rate repository with exclusive default ([a966f34](https://github.com/dknathalage/tallyo/commit/a966f34d459fb0036ae9db48eefb46ab8ece8026))
* **service:** rate tier + payer services with SSE broadcast ([ac7934f](https://github.com/dknathalage/tallyo/commit/ac7934f23432622fc560f12a59d6b57be7e4b41a))
* **service:** tax rate + client + catalog services with broadcast ([bd45b9f](https://github.com/dknathalage/tallyo/commit/bd45b9f080c7b0e9f471567a4ec256f99f5aaf0f))
* **service:** thread ctx and broadcast business_profile changes after commit ([efe8953](https://github.com/dknathalage/tallyo/commit/efe895320ecc081fff3b0c38755e0551e3efab45))
* **web:** createCollectionStore rune factory wired to SSE ([1faf832](https://github.com/dknathalage/tallyo/commit/1faf83259fbbe1403c9460f54d040c20bf552668))
* **web:** download invoice + estimate PDFs ([923ffde](https://github.com/dknathalage/tallyo/commit/923ffde5b8883154f01c28316ba7c2dbf41917fb))
* **web:** estimates UI with convert-to-invoice ([f4a6443](https://github.com/dknathalage/tallyo/commit/f4a64431bc42e95e26c496a5edae0a95836ee095))
* **web:** export buttons, column mappings, and catalog import wizard ([fc07b03](https://github.com/dknathalage/tallyo/commit/fc07b03b135e144306fcdb4fa7e88b602685424b))
* **web:** generic CRUD API helper for domain stores ([72ab4e6](https://github.com/dknathalage/tallyo/commit/72ab4e672b41978ffaf52298b74a9e8a293d9b53))
* **web:** invoices UI with line items, totals, and status ([718aff1](https://github.com/dknathalage/tallyo/commit/718aff1395a8878acee81b36e328cd7150dde871))
* **web:** rate tiers + payers UI with live SSE collection stores ([a9d8559](https://github.com/dknathalage/tallyo/commit/a9d8559425370d54dfc8664d4ccd215f8daabe1b))
* **web:** record payments and show invoice balance ([d2e9f57](https://github.com/dknathalage/tallyo/commit/d2e9f57f8846e261dbb8fcddc38ee2a996760d4e))
* **web:** recurring templates UI with generate-now ([3080879](https://github.com/dknathalage/tallyo/commit/308087908cb56a616bcac6e5638af4eb2ac5b0ec))
* **web:** SvelteKit SPA with auth, settings slice, invites, and SSE sync ([940d4b7](https://github.com/dknathalage/tallyo/commit/940d4b77d15f43153466c5405001a64fbfd04575))
* **web:** tax rates + clients + catalog UI ([e241731](https://github.com/dknathalage/tallyo/commit/e2417310ac025e560439a6c243c06ff4480c623e))


### Bug Fixes

* **auth:** atomic invite acceptance with dup-email 409 ([129dc24](https://github.com/dknathalage/tallyo/commit/129dc24501b79426511493b77de1c72fd120ffe3))
* **db:** cast ClientInvoiceStats total to REAL for float64 gen type ([006ca62](https://github.com/dknathalage/tallyo/commit/006ca62aed7d52ce7311f69f4ddf3965cfb5f3a1))
* **db:** immediate-write transactions so numbering is collision-free without -race ([f635582](https://github.com/dknathalage/tallyo/commit/f6355829cea99e665db9e02f86a30e60bd4af364))
* **web:** nav overflow — wider container, flex-wrap, nowrap links, gap after brand ([622fbd6](https://github.com/dknathalage/tallyo/commit/622fbd62643b4903740a1fa87308f4d0d774f771))

## [3.4.0](https://github.com/dknathalage/tallyo/compare/v3.3.2...v3.4.0) (2026-05-03)


### Features

* add AI chat assistant with skills, sub-agents, and audit log ([e892007](https://github.com/dknathalage/tallyo/commit/e892007a0fc25505d34ce59d4bcd86dc71a4e563))

## [3.3.2](https://github.com/dknathalage/tallyo/compare/v3.3.1...v3.3.2) (2026-05-02)


### Bug Fixes

* surface delete errors and dispatch invoice bulk actions ([fcaca72](https://github.com/dknathalage/tallyo/commit/fcaca72e0d48f1acfffa936959ed06a27cfc6c52))

## [3.3.1](https://github.com/dknathalage/tallyo/compare/v3.3.0...v3.3.1) (2026-05-02)


### Bug Fixes

* unwrap paginated API responses in invoice/estimate forms and CSV ops ([1a86cea](https://github.com/dknathalage/tallyo/commit/1a86cea9cdb68c561b9aafb3cf17263bad5da1a9))

## [3.3.0](https://github.com/dknathalage/tallyo/compare/v3.2.0...v3.3.0) (2026-05-02)


### Features

* new Tallyo logo (gradient T mark) ([7e9cbaa](https://github.com/dknathalage/tallyo/commit/7e9cbaa58f3557a565b09debe42e67576e977bea))

## [3.2.0](https://github.com/dknathalage/tallyo/compare/v3.1.0...v3.2.0) (2026-05-02)


### Features

* remove AI assistant feature ([bde4111](https://github.com/dknathalage/tallyo/commit/bde4111f55d5030a2ebf1b42097e32be118db4c6))

## [3.1.0](https://github.com/dknathalage/tallyo/compare/v3.0.0...v3.1.0) (2026-05-02)


### Features

* **electron:** brand the app with a Tallyo icon ([efdccc3](https://github.com/dknathalage/tallyo/commit/efdccc3ef64a10fe9c5400c01cdd1b6e2921512d))


### Bug Fixes

* **electron:** unpack node_modules so server bundle resolves deps ([a050bb8](https://github.com/dknathalage/tallyo/commit/a050bb8e2884561abd051dea8bf70ec4ae0e916b))
* **install:** repair dmg mount + add download progress bar ([086c5b6](https://github.com/dknathalage/tallyo/commit/086c5b6c9521c55dc6ec4da518639e34a551a8c3))

## [3.0.0](https://github.com/dknathalage/tallyo/compare/v2.1.1...v3.0.0) (2026-05-02)


### ⚠ BREAKING CHANGES

* Electron desktop app + GitHub Pages docs
* Electron desktop app + GitHub Pages docs
* `npm install -g tallyo` and the `tallyo` CLI are gone. Use the platform installer from the GitHub Release.

### Features

* drop CLI distribution, ship desktop only ([9830c77](https://github.com/dknathalage/tallyo/commit/9830c771c6061c0747dd315bf3e46bcb408479c8))
* Electron desktop app + GitHub Pages docs ([fefa404](https://github.com/dknathalage/tallyo/commit/fefa4040ea5b8255dad439861def829d4019d656))
* Electron desktop app + GitHub Pages docs ([fefa404](https://github.com/dknathalage/tallyo/commit/fefa4040ea5b8255dad439861def829d4019d656))
* ship Tallyo as Electron desktop app ([59fa205](https://github.com/dknathalage/tallyo/commit/59fa2059dbd11d34d6c08ad19252b48ed1da2e2f))
* split docs into static GitHub Pages site ([ee112c9](https://github.com/dknathalage/tallyo/commit/ee112c94ce37583d49b7a762c9ce157fa9eec612))

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
