import js from '@eslint/js';
import tseslint from 'typescript-eslint';
import svelte from 'eslint-plugin-svelte';
import globals from 'globals';

// Static-analysis ruleset adapted from NASA's "Power of Ten" for TS/Svelte.
// Rule numbers in comments map to CLAUDE.md "Coding Rules (NASA Power of 10)".
export default tseslint.config(
	{
		ignores: [
			'build/**',
			'.svelte-kit/**',
			'node_modules/**',
			'drizzle/**',
			'static/**',
			'coverage/**',
			'bin/**',
			'*.config.js',
			'*.config.ts'
		]
	},
	js.configs.recommended,
	...tseslint.configs.recommendedTypeChecked,
	...tseslint.configs.stylisticTypeChecked,
	...svelte.configs['flat/recommended'],
	{
		languageOptions: {
			globals: { ...globals.node, ...globals.browser },
			parserOptions: {
				projectService: true,
				extraFileExtensions: ['.svelte'],
				tsconfigRootDir: import.meta.dirname
			}
		},
		rules: {
			// Rule 1: simple control flow
			'no-labels': 'error',
			'no-label-var': 'error',
			'no-unreachable-loop': 'error',
			complexity: ['error', { max: 15 }],
			'max-depth': ['error', 4],
			'max-nested-callbacks': ['error', 4],

			// Rule 2: bounded loops
			'no-unmodified-loop-condition': 'error',

			// Rule 4: short functions (≤60 lines, ≤4 params)
			'max-lines-per-function': [
				'error',
				{ max: 60, skipBlankLines: true, skipComments: true, IIFEs: true }
			],
			'max-params': ['error', 4],
			'max-statements': ['error', 30],

			// Rule 5: assertion density / no silent coercion
			eqeqeq: ['error', 'always'],
			'no-implicit-coercion': 'error',
			'no-empty': ['error', { allowEmptyCatch: false }],
			'@typescript-eslint/no-non-null-assertion': 'error',

			// Rule 6: smallest scope
			'prefer-const': 'error',
			'no-var': 'error',
			'block-scoped-var': 'error',
			'no-shadow': 'off',
			'@typescript-eslint/no-shadow': 'error',

			// Rule 7: check every return value
			'@typescript-eslint/no-floating-promises': 'error',
			'@typescript-eslint/no-misused-promises': 'error',
			'@typescript-eslint/await-thenable': 'error',
			'@typescript-eslint/no-unused-vars': [
				'error',
				{ argsIgnorePattern: '^_', varsIgnorePattern: '^_', caughtErrorsIgnorePattern: '^_' }
			],
			'no-unused-expressions': 'off',
			'@typescript-eslint/no-unused-expressions': 'error',
			'require-atomic-updates': 'error',
			'no-promise-executor-return': 'error',
			'no-return-assign': 'error',

			// Rule 8: no metaprogramming / dynamic eval
			'no-eval': 'error',
			'no-implied-eval': 'error',
			'no-new-func': 'error',
			'no-script-url': 'error',

			// Rule 9: limit indirection
			'max-lines': ['error', { max: 500, skipBlankLines: true, skipComments: true }],

			// Rule 10: compile clean at max strictness
			'@typescript-eslint/no-explicit-any': 'error',
			'@typescript-eslint/ban-ts-comment': [
				'error',
				{
					'ts-expect-error': 'allow-with-description',
					'ts-ignore': 'allow-with-description',
					'ts-nocheck': true,
					minimumDescriptionLength: 10
				}
			],
			'@typescript-eslint/no-unnecessary-condition': 'error',
			'@typescript-eslint/no-unnecessary-type-assertion': 'error',
			'@typescript-eslint/prefer-nullish-coalescing': 'error',
			'@typescript-eslint/switch-exhaustiveness-check': 'error',

			// extra bug-catchers
			'no-console': ['warn', { allow: ['warn', 'error', 'info'] }],
			'no-constant-binary-expression': 'error',
			'no-self-compare': 'error',
			'no-template-curly-in-string': 'error',
			'array-callback-return': 'error',
			'consistent-return': 'error',

			// Type-flow noise from Drizzle/SvelteKit inference: surface as warnings
			// so adoption is incremental. Tighten to 'error' once cleaned up.
			'@typescript-eslint/no-unsafe-argument': 'warn',
			'@typescript-eslint/no-unsafe-assignment': 'warn',
			'@typescript-eslint/no-unsafe-call': 'warn',
			'@typescript-eslint/no-unsafe-member-access': 'warn',
			'@typescript-eslint/no-unsafe-return': 'warn',
			'@typescript-eslint/unbound-method': 'warn',
			'@typescript-eslint/require-await': 'warn',
			'@typescript-eslint/only-throw-error': 'warn',
			'@typescript-eslint/no-redundant-type-constituents': 'warn',
			'@typescript-eslint/no-base-to-string': 'warn',
			'@typescript-eslint/no-empty-function': 'warn',
			'svelte/no-navigation-without-resolve': 'warn',
			'svelte/require-each-key': 'warn',
			'svelte/prefer-svelte-reactivity': 'warn'
		}
	},
	{
		// .svelte.ts (Svelte 5 rune store files) must use the TS parser, not the
		// Svelte parser — they are plain TypeScript modules.
		files: ['**/*.svelte.ts'],
		languageOptions: {
			parser: tseslint.parser,
			parserOptions: {
				projectService: true
			}
		}
	},
	{
		// Svelte files: parser handles <script> blocks
		files: ['**/*.svelte'],
		languageOptions: {
			parserOptions: {
				parser: tseslint.parser,
				projectService: true,
				extraFileExtensions: ['.svelte']
			}
		},
		rules: {
			// Components often exceed 60 lines of template + script combined
			'max-lines-per-function': 'off'
		}
	},
	{
		// Tests can be longer + use any for fixtures
		files: ['**/*.test.ts', '**/*.spec.ts'],
		rules: {
			'max-lines-per-function': 'off',
			'max-statements': 'off',
			'@typescript-eslint/no-explicit-any': 'off',
			'@typescript-eslint/no-non-null-assertion': 'off'
		}
	},
	{
		// CLI entry uses console + process — Node script
		files: ['bin/**/*.js'],
		rules: {
			'no-console': 'off',
			'@typescript-eslint/no-floating-promises': 'off'
		}
	}
);
