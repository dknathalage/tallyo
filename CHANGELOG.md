# Changelog

## [3.7.0](https://github.com/dknathalage/tallyo/compare/v3.6.0...v3.7.0) (2026-06-24)


### Features

* **agent:** add propose[T] forced-tool structured-output helper ([a23f50e](https://github.com/dknathalage/tallyo/commit/a23f50eeb9df3a4a2d770514458249de8d81791a))
* **agent:** agent tables, sqlc queries, store ([4faa11f](https://github.com/dknathalage/tallyo/commit/4faa11fa4c986c6acd7837ee954b901a5edb9f7b))
* **agent:** anthropic-sdk-go adapter for llm.Client ([e3232ec](https://github.com/dknathalage/tallyo/commit/e3232ec111bb7f531fd7867f6d2c053aacf32874))
* **agent:** bounded execute loop with auto-run read tools ([bd5d8f1](https://github.com/dknathalage/tallyo/commit/bd5d8f1973488c073773c5277e752c68af9d19cd))
* **agent:** catalogue-authoritative pricing and notes-completeness verify ([843d904](https://github.com/dknathalage/tallyo/commit/843d904b2b7e533ad650bdd09629c35ae28f149f))
* **agent:** config + anthropic-sdk-go dependency ([7a939d1](https://github.com/dknathalage/tallyo/commit/7a939d16a8e0f9d84d589818f74ede0b8ec7668a))
* **agent:** create_invoice keyed on shifts (verify + bill) ([79e695a](https://github.com/dknathalage/tallyo/commit/79e695ac4330ed98163ce30897def19a2c380d65))
* **agent:** DivideShift — AI divides one shift into priced items ([999d893](https://github.com/dknathalage/tallyo/commit/999d89329c11de0ec3b83d2839934671cc10e2c1))
* **agent:** DivideShift route; remove whole-invoice draft Smart route ([d596a03](https://github.com/dknathalage/tallyo/commit/d596a036dbb0037738730ca2f952de6481dd205b))
* **agent:** draft Smart grounds itself via catalogue search; narrative line items ([ed9bfc4](https://github.com/dknathalage/tallyo/commit/ed9bfc4e5b523f6b3042062e34f0e002dbf13451))
* **agent:** draft-invoice Smart (gather → propose → apply, bounded retry) ([a4965fb](https://github.com/dknathalage/tallyo/commit/a4965fbf1e7d3e6c3d67c535dba10024a1b6dbb4))
* **agent:** expose checkpoint id on messages; web RevertControl ([1654171](https://github.com/dknathalage/tallyo/commit/1654171b4815cca4cd3d565bc70fd51db91c08c5))
* **agent:** forced propose_plan phase ([66c9b1e](https://github.com/dknathalage/tallyo/commit/66c9b1eb771388d900995dba996c5f668af2fa49))
* **agent:** hardened system prompt + untrusted-content wrapping ([9beed56](https://github.com/dknathalage/tallyo/commit/9beed5632017bfa79c9d5d9b5a46e59c3a01556a))
* **agent:** HTTP endpoints + per-conversation SSE stream ([e788c31](https://github.com/dknathalage/tallyo/commit/e788c31a0d0b82c44fe15d9d39f317a53f41461c))
* **agent:** import creates note-only shifts (quantities folded into note) ([40735ee](https://github.com/dknathalage/tallyo/commit/40735ee4c3d57d1a3592da1db24442c8cb3c97de))
* **agent:** list_participant_shifts tool + candidates ([13c35d3](https://github.com/dknathalage/tallyo/commit/13c35d3a60b3e1c1cda1e19416333cdbfd1a7863))
* **agent:** per-tenant daily token cap + per-user rate limit ([5aae271](https://github.com/dknathalage/tallyo/commit/5aae271d92ea2a4a02a793ea451d1bc1c997df89))
* **agent:** per-turn checkpoint snapshot + revert; risky create_invoice ([c8413a1](https://github.com/dknathalage/tallyo/commit/c8413a1563cee4ef0bd2f12c031b8f3da132a62e))
* **agent:** provider-agnostic llm client interface + scripted fake ([17959bc](https://github.com/dknathalage/tallyo/commit/17959bc5d97d354a20fe61370dc67b947199c2fd))
* **agent:** resumable risky-op permission gate ([29dd9a7](https://github.com/dknathalage/tallyo/commit/29dd9a77f027cb6000b9758eefa0915514a6215e))
* **agent:** SmartsHandler + Smarts.ImportShifts (additive; relocate shared helpers) ([fc8a9be](https://github.com/dknathalage/tallyo/commit/fc8a9bea9f993aa41849d9ccae92e791481650a1))
* **agent:** stall recovery, skip-plan, and pre-loaded candidate codes ([1f8de81](https://github.com/dknathalage/tallyo/commit/1f8de819f4a954f98a77cfb7e27b52bfb8709bcb))
* **agent:** timesheet text → shifts extraction ([f85c10f](https://github.com/dknathalage/tallyo/commit/f85c10fc765151abf8585f50d60f2512eae21c80))
* **agent:** tool registry + list_invoices read tool ([9e455aa](https://github.com/dknathalage/tallyo/commit/9e455aa9fae086be7630cfafdc2ce3bc2d7f04ee))
* **agent:** wire agent service + expired-step/retention sweep ([7cf44d2](https://github.com/dknathalage/tallyo/commit/7cf44d22a77950e44343772b9a8040322fd61791))
* **ai:** add ai feature ([8ef0257](https://github.com/dknathalage/tallyo/commit/8ef025795e6dc17f79b5ce2b385ccf50987ca7be))
* **app:** /auth/session + tenant-scoped /api/t/{tenantUUID} router group ([46670d7](https://github.com/dknathalage/tallyo/commit/46670d70de847cfe3dd1a2f93151aa54c19f303d))
* **auth:** persist email in session (login+signup) for per-request tenant auth ([bbee347](https://github.com/dknathalage/tallyo/commit/bbee347778cf1c610781a39bd45874e5c967d978))
* **auth:** self-serve signup, roles, fail-safe multi-tenant login, suspended guard ([d8d6495](https://github.com/dknathalage/tallyo/commit/d8d649567e4f31b14316e4ed5a20f182e3e54f4c))
* **auth:** TenantsRepo.GetByUUID for url-tenant resolution ([f52b0bc](https://github.com/dknathalage/tallyo/commit/f52b0bc48219d74f1b3fbddb83cc38e8058e8266))
* **billing,client:** data-presence validation gating + client.type field gating ([6c52495](https://github.com/dknathalage/tallyo/commit/6c52495360056f493f690e2b745786205bb1e3d1))
* **billing:** add shared LineItem(Input); repoint all references ([73ad04e](https://github.com/dknathalage/tallyo/commit/73ad04ed3563b80b735c0ccb0f0e5d66541a7ec4))
* **billing:** unit class classifier (time/distance/count) ([4d0cb1f](https://github.com/dknathalage/tallyo/commit/4d0cb1f33f4c9913565e748f94aade8abd8d68ad))
* **catalog:** platform-admin NDIS catalogue XLSX ingest ([aad0fab](https://github.com/dknathalage/tallyo/commit/aad0fabef38b9c4614597025f1a55c275dd97367))
* **catalog:** seed NDIS catalogue via versioned migration; freeze invoice prices ([cb9c7c9](https://github.com/dknathalage/tallyo/commit/cb9c7c9530062549b65abb825ec37290c4a44dc4))
* cross-DB signup provisioning + sweep wiring (Phase 6/7) ([6bff8c1](https://github.com/dknathalage/tallyo/commit/6bff8c13e1989f603b218e6965dfdd291ec6b678))
* **db:** drop agent chat/checkpoint tables; remove agent queries ([c0c0ed7](https://github.com/dknathalage/tallyo/commit/c0c0ed74ee363c541ca362f46e2fafbc875bb6ff))
* **db:** fresh NDIS multi-tenant schema baseline ([deddbbb](https://github.com/dknathalage/tallyo/commit/deddbbbc35ede42652f9ea32788319a369f1a235))
* **db:** migration 00008 — unify shift items into line_items ([750c1d2](https://github.com/dknathalage/tallyo/commit/750c1d2bcb0043c220b5f92e0ee5f6bf2a6b2187))
* **db:** shift-scoped line_item queries + drop shift quantity cols; billing line item shift/time fields ([1480003](https://github.com/dknathalage/tallyo/commit/14800030ee046e7a8479566afdc9e5cabe88a21a))
* **db:** shifts table + queries (alongside notes) ([26f8b48](https://github.com/dknathalage/tallyo/commit/26f8b4863f1b5c4cbdc074d9bdc5cd5d8653dd19))
* **db:** tenant-scoped sqlc queries for NDIS schema ([38c3f48](https://github.com/dknathalage/tallyo/commit/38c3f483e69c92fd22fa7bbd136a66e67ed0b2c0))
* gate AI Smarts behind TALLYO_FEATURE_AGENT; drop export slice ([dd00d17](https://github.com/dknathalage/tallyo/commit/dd00d1738e71e9f62b5d65131a671763e7e1634a))
* **http:** shift CRUD + filters + status ([8fe07b0](https://github.com/dknathalage/tallyo/commit/8fe07b0e2df63ec0b4e53502d6931cfb2923a61c))
* **http:** shift import + draft-invoice + wire ShiftService/tools ([a82cbc6](https://github.com/dknathalage/tallyo/commit/a82cbc6dbd09f174a6a788f65744a74c38a6cbc3))
* **httpx:** add ParseUUID path-param helper ([ab0d9c2](https://github.com/dknathalage/tallyo/commit/ab0d9c299cdd737364f8c59ab636a80b00b3e119))
* **httpx:** RequireSession + ResolveTenant middleware (url tenant authz by email) ([1cbaae8](https://github.com/dknathalage/tallyo/commit/1cbaae835e887ee3f01c4c4fafbb698e408b7e00))
* **importer:** auto-detect catalog column mapping from headers + values ([715328a](https://github.com/dknathalage/tallyo/commit/715328a58b1e3fffde150e83b6ae62a7e3ecb1eb))
* **importer:** transient mapping with name-keyed tiers, create-if-missing commit ([2303291](https://github.com/dknathalage/tallyo/commit/230329151b26770fe566c778c25e02ab169be0bb))
* **invoice:** deterministic DraftFromShifts (link priced items) ([ead06d0](https://github.com/dknathalage/tallyo/commit/ead06d06302d1ab4ea03ae748080977ac7c7b280))
* **invoice:** NDIS line validation engine (price-cap, plan-window, gst-free) ([ac5db77](https://github.com/dknathalage/tallyo/commit/ac5db77dafdd7bf913a99b80a7576ca9d5f3db1e))
* **listquery:** safe clause builder with injection safeguards + tests ([056eaf2](https://github.com/dknathalage/tallyo/commit/056eaf2e1be84c509e0d1bd14a140a8745d7f01d))
* make NDIS pricing zone optional; price generic coded lines from items.unit_price ([cfef632](https://github.com/dknathalage/tallyo/commit/cfef63275aba14634184c5eae99d79eb3cbfc3b6))
* **participant:** {rows,total} list query endpoint via listquery ([8cd5d72](https://github.com/dknathalage/tallyo/commit/8cd5d7281e0d47fc8464e980f16046c67dc68ad7))
* **pricelist:** generic upload-and-map import; remove NDIS XLSX parser ([8b02c81](https://github.com/dknathalage/tallyo/commit/8b02c81d66977030823ba6fff4ce7f869c9d8eaf))
* **repo:** ShiftsRepo lifecycle + JSON measures/tags ([5744b4f](https://github.com/dknathalage/tallyo/commit/5744b4fa587aa2e6254a866e5d2e7ae7e696c8eb))
* **repo:** ShiftsRepo lifecycle + JSON measures/tags ([f645e3e](https://github.com/dknathalage/tallyo/commit/f645e3e1d9e2e1026c2729c8e5f6a2d21528f999))
* reshape service/http/pdf/export for NDIS multi-tenant; restore green build ([25639fa](https://github.com/dknathalage/tallyo/commit/25639fa245cd0629dd817db7827c72fd5e70b8f9))
* **service:** cascade invoice status to shifts ([7840d4c](https://github.com/dknathalage/tallyo/commit/7840d4c3dff09396cf41ec1afbcb2e5d0db3e7a3))
* **service:** ShiftService + suggestions + lifecycle ([8f2521c](https://github.com/dknathalage/tallyo/commit/8f2521c0a219410a97458b8c1a0f57f6248e3684))
* **service:** ShiftService + suggestions + lifecycle ([227aae9](https://github.com/dknathalage/tallyo/commit/227aae994cc9465c1a8f566e14c1046bf3b5a107))
* **shift:** shift-scoped line items (repo + service + handler) ([b84f5fd](https://github.com/dknathalage/tallyo/commit/b84f5fd0ca35707a6be274356db79ce90300d8ef))
* structured logging with log/slog ([0dd9145](https://github.com/dknathalage/tallyo/commit/0dd9145b228d8103a21c90b5b093f42819786003))
* tenant context helpers + reshape repository & auth layers ([de0e48d](https://github.com/dknathalage/tallyo/commit/de0e48db032dbee94966fc29f4cf0741ceaa8440))
* tenant-scoped SSE + audit stamping + numbering consolidation + per-tenant sweeps ([b8508c2](https://github.com/dknathalage/tallyo/commit/b8508c24add3ae8f2ef588f760e4523a917fdd12))
* **tenantdb:** per-tenant SQLite handle registry (Phase 4) ([c626741](https://github.com/dknathalage/tallyo/commit/c626741988201335bbb960c15182fab4abc3e3b4))
* ui redone ([fa0b932](https://github.com/dknathalage/tallyo/commit/fa0b93222cccf7d8f38027120920d70856c5e81c))
* **web:** active-tenant API client + per-request tenant-prefixed crud paths ([04cb056](https://github.com/dknathalage/tallyo/commit/04cb0569b341157eeb703bbeafcc26b7bac7df81))
* **web:** add manual Save button to EntityEditor as autosave safety net ([55be26b](https://github.com/dknathalage/tallyo/commit/55be26b020545cabf73fd2c245c6cfc8b18f982b))
* **web:** add optional edit-input hint to DataTable Column ([bbf629f](https://github.com/dknathalage/tallyo/commit/bbf629f6447b86b0f2dba6fc12710a3f2e2477a9))
* **web:** agent API client + types ([1ec796f](https://github.com/dknathalage/tallyo/commit/1ec796f09a902fda65fb0abdcdda266d9c6aa35d))
* **web:** agentChat runes store + event reducer ([ddf626d](https://github.com/dknathalage/tallyo/commit/ddf626d3d781604ec84bb1abe976bcf4d2deaa2d))
* **web:** bespoke participant edit/new form with plan-manager select (createAutosave) ([5267256](https://github.com/dknathalage/tallyo/commit/5267256a1e3da9ddf1bf3d8ce7b80cc5da13c064))
* **web:** calendar + participant profile ([5cba711](https://github.com/dknathalage/tallyo/commit/5cba711caa7554b17df8fe71460bca256eaee095))
* **web:** chat pane assembly ([634f212](https://github.com/dknathalage/tallyo/commit/634f2126dbd81f4cd2c580bdad29205a52867e12))
* **web:** chat shell components (conversation list, composer, message bubble) ([8c6b988](https://github.com/dknathalage/tallyo/commit/8c6b988931ea0f42d408644a9551c2b19aa3aa82))
* **web:** chat-first home route, assistant nav entry, global shortcuts ([208239e](https://github.com/dknathalage/tallyo/commit/208239ea13a839a60fcef2881efaa65b36f6cf51))
* **web:** custom-items autosaving edit page ([3b95b85](https://github.com/dknathalage/tallyo/commit/3b95b85ea98ccfedf763d25eecc37c99cb2e24f5))
* **web:** DataTable navigates to edit page instead of opening drawer ([ce9bd79](https://github.com/dknathalage/tallyo/commit/ce9bd79bc7efd8043b8e4e3e075482224acd5494))
* **web:** debounced single-in-flight autosave state machine ([a560225](https://github.com/dknathalage/tallyo/commit/a5602251fd4426e72499b2d2af298ff93fb65abf))
* **web:** estimates row click opens edit page (EntityEditor + extras) ([815737e](https://github.com/dknathalage/tallyo/commit/815737efda171bef57a8a125acd9e5cd99c5c04d))
* **web:** generic DataTable component (sort/filter/select/drawer) ([6e8d64c](https://github.com/dknathalage/tallyo/commit/6e8d64c342f1f649cbcf8a28768cd8da050a4662))
* **web:** generic EntityEditor edit/new page with autosave ([784396e](https://github.com/dknathalage/tallyo/commit/784396ee778e780183b595a26e4dba6d82482d1a))
* **web:** inline catalog import wizard; remove column-mappings UI ([073302d](https://github.com/dknathalage/tallyo/commit/073302dd9ded69ac05e151b2fa0a247dcba044d2))
* **web:** invoice detail with source shifts + status actions ([38c20dd](https://github.com/dknathalage/tallyo/commit/38c20dd1f5174d26fbcd78c1a513a5f97f725a2f))
* **web:** invoices row click opens edit page (EntityEditor + extras) ([0f86436](https://github.com/dknathalage/tallyo/commit/0f86436ce336c8e27ec4482627385c53e54c924a))
* **web:** migrate tax-rates, custom-items, plan-managers, invoices, estimates, recurring to DataTable ([09154cf](https://github.com/dknathalage/tallyo/commit/09154cf31a114ff9d0da56120e69b2ed1b549b8b))
* **web:** move authed routes under [tenant]; tenant layout, switcher, t() link prefixing ([7539f05](https://github.com/dknathalage/tallyo/commit/7539f0587d834448e545a28699de87bcbf48bdc2))
* **web:** move calendar into shifts page (table/calendar toggle); remove /calendar route ([f765620](https://github.com/dknathalage/tallyo/commit/f7656209a8ed61bff2a628a6daa5f46f3406551c))
* **web:** nav redesign, dark mode, and modal create forms ([3388b97](https://github.com/dknathalage/tallyo/commit/3388b97dff17bbf35595f1ce9e5f81b5ac24f58d))
* **web:** NDIS multi-tenant SPA — signup, participants, support catalogue, validation ([528b300](https://github.com/dknathalage/tallyo/commit/528b3004f77e2107ca3569fc488ef5449347d95a))
* **web:** participants list on DataTable + align listquery keys ([40c6923](https://github.com/dknathalage/tallyo/commit/40c692328ec59a5423b901a7ee91b4508acaf317))
* **web:** participants row click opens edit page (EntityEditor + shifts extras) ([7ca94d3](https://github.com/dknathalage/tallyo/commit/7ca94d3d898359590e3880775b288986efe186e7))
* **web:** plan-managers autosaving edit page ([c568c57](https://github.com/dknathalage/tallyo/commit/c568c573956cd622274411c536fe1ad017166673))
* **web:** recurring row click opens edit page (relational form on route) ([6b1a492](https://github.com/dknathalage/tallyo/commit/6b1a49296215b1c4129fc8f3833953e48f465d2c))
* **web:** route all tenant-scoped fetches through tenantPath; split session store; SSE close/reopen ([2a752e7](https://github.com/dknathalage/tallyo/commit/2a752e76c731be36da85d4db176de0d712a3350f))
* **web:** server-side list query in crud + collection store ([66c598a](https://github.com/dknathalage/tallyo/commit/66c598a761278b6ec43809387f4a41d12d9990c5))
* **web:** shift edit/new as a full route page; home list navigates (modal removed) ([c455dd9](https://github.com/dknathalage/tallyo/commit/c455dd9aecaaa6e65e84c89da5af15353c48805c))
* **web:** shift item types + client; draft-from-shifts action ([739c3e0](https://github.com/dknathalage/tallyo/commit/739c3e0f6b1550fcb2d583b9f44a919ac0f73f76))
* **web:** shift items editor + divide-with-AI; sweep dropped fields ([0d47e3c](https://github.com/dknathalage/tallyo/commit/0d47e3c10a57af4c514cf4d41f8f2340c5134714))
* **web:** shifts api + rune store + types ([e7b0445](https://github.com/dknathalage/tallyo/commit/e7b0445222b6250de225dec79b9dbbc244d26050))
* **web:** shifts home (pipeline, to-record, quick-add, suggestions, table) ([34421f6](https://github.com/dknathalage/tallyo/commit/34421f6f941f46819bad05cc1ba4a1fb2f5158eb))
* **web:** shifts home + table + recording form ([117af5c](https://github.com/dknathalage/tallyo/commit/117af5cdf1b032db8720333f36f7f22b0e19a43d))
* **web:** tax-rates row click opens autosaving edit page ([83f0dad](https://github.com/dknathalage/tallyo/commit/83f0dad881481f83f0d95640f1b00c0327d7df59))
* **web:** turn-rendering components (plan card, tool-result renderers, access prompt) ([0b5d931](https://github.com/dknathalage/tallyo/commit/0b5d93195150a73f1081aea6c595083498d9bd7d))
* **web:** typed agent SSE event union + per-conversation stream client ([3a872e8](https://github.com/dknathalage/tallyo/commit/3a872e86e07797209a24c96c2be512e46dacc04b))


### Bug Fixes

* add taskfile ([1d2b5a1](https://github.com/dknathalage/tallyo/commit/1d2b5a17bce09bac780209c18699ff3adc4d292e))
* **agent/llm:** streaming, thinking gate, and prompt caching ([931bb48](https://github.com/dknathalage/tallyo/commit/931bb488eccc14901a36dce1165fe0b0544ae979))
* **agent:** balance plan tool_use window, registry-derived risk, 503 when disabled ([5e2268f](https://github.com/dknathalage/tallyo/commit/5e2268f4f37ddaa684a1ee61644276e925e49045))
* **agent:** draft propose loop — fail fast on refusal/max-tokens; skip discarded searches ([a0674af](https://github.com/dknathalage/tallyo/commit/a0674afa2d00bd6b2ca84ee768e777c9856bdef6))
* **agent:** draft-invoice returns the create_invoice RESULT, not the 'Run …' summary ([256cc8a](https://github.com/dknathalage/tallyo/commit/256cc8acd67479c44bf7406d26931fde4d32235e))
* **agent:** monotonic checkpoint ordinal for multi-step revert ([b7bb037](https://github.com/dknathalage/tallyo/commit/b7bb03744053f923e8e3dd6c53ba8d0363f20779))
* **agent:** propagate tool_result/history persistence errors; guard sentinel names ([d1ff9bc](https://github.com/dknathalage/tallyo/commit/d1ff9bc3e086038831844aa69bca927aebaea895))
* **agent:** typed recoverable-error sentinel for draft retry; tighten tests ([55c401d](https://github.com/dknathalage/tallyo/commit/55c401d3f56ca3c18742899122b18a3983da07d6))
* **api:** bulk-delete by uuid for participant/plan-manager/custom-item ([909122e](https://github.com/dknathalage/tallyo/commit/909122e64859edf6964cd12b4c59e13c62a75b18))
* **auth,repo:** narrow unique-violation match; tenant-scope SetRecurringNextDue ([0691b5f](https://github.com/dknathalage/tallyo/commit/0691b5f18f0fed0565593c757c4e86bf69611194))
* **auth:** login multi-tenant re-submit by tenant uuid ([a23b5bc](https://github.com/dknathalage/tallyo/commit/a23b5bc4b197973458c64d6a132a32307681228a))
* **auth:** normalize login email so registered users can sign in ([c6ab8c3](https://github.com/dknathalage/tallyo/commit/c6ab8c3188e239098dfc5dd7135a85523f01f3ac))
* **billing:** keep Round2 verbatim (math.Round(x*100)/100); drop unneeded epsilon ([9468086](https://github.com/dknathalage/tallyo/commit/94680868fee58379057cfbcbc2430b1de6e6c72c))
* **billing:** read price list from tenant DB, not control, in LineValidator ([d07c634](https://github.com/dknathalage/tallyo/commit/d07c6349e073233ad34187efdb2b8842f9abfa75))
* **catalog:** capture national price from per-state columns (no National column in NDIS sheet) ([2fac3a8](https://github.com/dknathalage/tallyo/commit/2fac3a89df56cb25577e707364126fb3b38609b1))
* **gen:** resync — control session model is Session (matches `sessions` table) ([a03dec1](https://github.com/dknathalage/tallyo/commit/a03dec18607267656f7cc101080b1cbfeaf4e856))
* **invoice:** both delete paths unlink shift items + revert shifts before cascade ([a4210be](https://github.com/dknathalage/tallyo/commit/a4210bef162319ce5329259080de33a79391709d))
* **invoice:** catalogue governs gst_free for support items (compliance) ([1ffbcb8](https://github.com/dknathalage/tallyo/commit/1ffbcb8764aecdcaf5fb82191dd624a7604f36f3))
* remove opensource license ([3e981c0](https://github.com/dknathalage/tallyo/commit/3e981c0baeb5d871336150a94b443d3308863807))
* **web:** disable composer while awaiting approval; robust cell formatting; scroll on status ([a6bccc2](https://github.com/dknathalage/tallyo/commit/a6bccc288fb750cc75852503f8a2c76bfe8ea659))
* **web:** genericize NDIS UI copy; make signup zone optional ([00a79f3](https://github.com/dknathalage/tallyo/commit/00a79f3a024ea6a0fd68607b447aa05aab3d7dcf))
* **web:** remount edit page on route id change so edits save to the right record ([86e98f3](https://github.com/dknathalage/tallyo/commit/86e98f37d80f3cda112efe5c81b9bab40b587b69))
* **web:** remove NDIS pricing-zone field from signup; generic payer example ([883e2b9](https://github.com/dknathalage/tallyo/commit/883e2b9f5db96e12c57f342139b5185b606d0eab))
* **web:** seed autosave with existing id so edits update instead of creating ([caebc69](https://github.com/dknathalage/tallyo/commit/caebc6954fde14db26dfd6e707ce5c3439308077))
* **web:** send fileType so XLSX imports parse correctly; a11y + cleanup ([5fd1095](https://github.com/dknathalage/tallyo/commit/5fd10951f13cf3594a8ad718b164e55aa6c5699b))
* **web:** set active tenant in [tenant] layout load to avoid tenantPath race ([c9f9076](https://github.com/dknathalage/tallyo/commit/c9f90769a835051a7be1cca3f15a79709db3b502))

## [3.6.0](https://github.com/dknathalage/tallyo/compare/v3.5.0...v3.6.0) (2026-06-05)


### Features

* add --version flag with ldflags-injected build version ([75c0370](https://github.com/dknathalage/tallyo/commit/75c037063a62e75a137b4f082f41f45fa2644170))

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
