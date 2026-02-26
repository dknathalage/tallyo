Commit all staged and unstaged changes, then push to remote.

Steps:
1. Run `git status` to see all changes (never use -uall flag)
2. Run `git diff` to see staged and unstaged changes
3. Run `git log --oneline -5` to see recent commit message style
4. Analyze all changes and draft a concise commit message that:
   - Summarizes the nature of the changes (feature, fix, refactor, etc.)
   - Focuses on the "why" not the "what"
   - Follows the existing commit style from the log
   - Does NOT commit files containing secrets (.env, credentials, etc.)
5. Stage all relevant files by name (not `git add -A`)
6. Commit using a HEREDOC for the message, ending with:
   `Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>`
7. Run `git status` to verify the commit succeeded
8. Push to the remote
