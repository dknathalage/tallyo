/**
 * Semantic Release configuration.
 * Automatically versions and releases based on conventional commits on main:
 *   feat:     → minor bump (1.0.x → 1.1.0)
 *   fix:      → patch bump (1.0.0 → 1.0.1)
 *   feat!: or BREAKING CHANGE → major bump (1.0.0 → 2.0.0)
 *   chore/docs/ci/refactor/test → no release
 */
export default {
  branches: ['main'],
  plugins: [
    // Determine release type from commit messages
    ['@semantic-release/commit-analyzer', {
      preset: 'conventionalCommits',
      releaseRules: [
        { type: 'feat',     release: 'minor' },
        { type: 'fix',      release: 'patch' },
        { type: 'perf',     release: 'patch' },
        { type: 'revert',   release: 'patch' },
        { breaking: true,   release: 'major' },
      ]
    }],

    // Generate changelog section of release notes
    ['@semantic-release/release-notes-generator', {
      preset: 'conventionalCommits',
      presetConfig: {
        types: [
          { type: 'feat',     section: '🚀 Features' },
          { type: 'fix',      section: '🐛 Bug Fixes' },
          { type: 'perf',     section: '⚡ Performance' },
          { type: 'refactor', section: '♻️ Refactor' },
          { type: 'docs',     section: '📚 Documentation' },
          { type: 'ci',       section: '🔧 CI/CD',     hidden: false },
          { type: 'chore',    section: '🧹 Chores',    hidden: true },
          { type: 'test',     section: '🧪 Tests',     hidden: true },
        ]
      }
    }],

    // Bump version in package.json (no npm publish)
    ['@semantic-release/npm', { npmPublish: false }],

    // Commit the package.json version bump back to main
    ['@semantic-release/git', {
      assets: ['package.json'],
      message: 'chore(release): ${nextRelease.version} [skip ci]\n\n${nextRelease.notes}'
    }],

    // Create GitHub Release
    ['@semantic-release/github', {
      successComment: false,
      failTitle: false,
    }],
  ]
};
