When fixing a bug or issue:

1. **Diagnose** — Read the relevant source files to understand the root cause before making changes.

2. **Fix** — Apply the minimal fix needed. Do not refactor surrounding code.

3. **Write regression tests** — After every fix, create or update tests that:
   - Reproduce the original failure scenario (the test should fail without the fix)
   - Verify the fix works for the exact case reported
   - Cover edge cases and related scenarios (e.g., empty inputs, error responses, boundary conditions)
   - Follow the existing test patterns in the codebase (co-located `.test.ts` files, vitest, vi.mock for dependencies)

4. **Run tests** — Run `npm test` to ensure all tests pass (existing + new).

5. **Run type check** — Run `npm run check` to ensure no TypeScript errors were introduced.

Always create tests. A fix without a test is incomplete.
