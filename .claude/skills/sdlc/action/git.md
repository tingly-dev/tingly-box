# Git Skill

The `/git` skill assists with Git operations for SDLC workflow including status checks, diffs, branch management, and workflow helpers.

## Usage

```
/git [operation] [args?]
```

**Operations:**
- `status` - Enhanced status information
- `diff` - Show changes with context
- `log` - Show commit history
- `branch` - Branch management operations
- `checkout` - Checkout branches or commits
- `merge` - Merge operations (see `/git-resolve` for conflicts)
- `clean` - Working directory cleanup
- `workflow` - Git workflow helpers (feature, bugfix, hotfix)

**Examples:**
- `/git status` - Show enhanced git status
- `/git diff` - Show unstaged changes
- `/git log --oneline -10` - Show last 10 commits
- `/git branch create feature/user-auth` - Create feature branch
- `/git checkout main` - Switch to main branch
- `/git workflow start feature/add-oauth` - Start feature workflow

## Guidelines

### When to Use
- **Before Starting Work**: Check status and pull latest changes
- **Branch Creation**: Create properly named branches for work
- **Code Review**: Check diff and prepare for PR creation
- **Before Merging**: Check branch status
- **Conflict Resolution**: Use `/git-resolve` for merge conflict assistance
- **Workflow Management**: Follow Git flow or GitHub flow conventions
- **History Review**: Check commit history and changes

### Git Safety Protocols

**Critical Rules:**
1. **NEVER update git config** without explicit request
2. **NEVER run destructive commands** (push --force, reset --hard, checkout ., restore ., clean -f, branch -D) without explicit request
3. **NEVER skip hooks** (--no-verify, --no-gpg-sign) without explicit request
4. **NEVER force push to main/master** - warn user if requested
5. **ALWAYS create NEW commits** rather than amending unless explicitly requested
6. **ALWAYS check for uncommitted changes** before destructive operations
7. **Prefer safer alternatives** to destructive commands

**Safe Alternatives:**
- Instead of `git reset --hard`: Use `git checkout` or specific file operations
- Instead of `git clean -f`: Use `git clean -f -d` with dry-run first
- Instead of `git push --force`: Use `git push --force-with-lease`

**Commit Safety:**
- After hook failure, create NEW commit (don't amend)
- Stage specific files by name, not `git add .` or `git add -A`
- Avoid committing secrets (.env, credentials.json)

### Branch Management

#### Create Branch
```
/git branch create <branch-name>
```

**Naming Conventions:**
- `feature/<name>` - New features
- `bugfix/<name>` - Bug fixes
- `hotfix/<name>` - Urgent production fixes
- `refactor/<name>` - Code refactoring
- `docs/<name>` - Documentation updates
- `test/<name>` - Test additions or changes

**Process:**
1. Ensure working directory is clean
2. Checkout base branch (usually `main` or `develop`)
3. Pull latest changes
4. Create new branch from base
5. Checkout new branch

#### List Branches
```
/git branch list [--all|--remote|--local]
```

Display branches with:
- Current branch indicator
- Last commit info
- Tracking branches
- Stale branches

#### Delete Branch
```
/git branch delete <branch-name> [--force]
```

**Process:**
1. Check if branch is merged
2. Warn if branch contains unmerged work
4. Delete branch locally and remotely

### Merge Operations

#### Prepare Merge
```
/git merge prepare <source-branch> <target-branch>
```

**Checks:**
- Working directory cleanliness
- Branch up-to-date status
- Potential conflict detection
- Commit message generation

#### Execute Merge
```
/git merge execute <source-branch> [target-branch]
```

**Process:**
1. Fetch latest changes
2. Checkout target branch
3. Pull latest changes
4. Merge source branch
5. If conflicts occur, use `/git-resolve` for assistance
6. Create merge commit
7. Push to remote

### Branch Management

#### Start Feature Workflow
```
/git workflow start feature <feature-name>
```

**Process:**
1. Ensure clean working directory
2. Checkout base branch
3. Pull latest changes
4. Create feature branch
5. Show workflow steps

#### Complete Feature Workflow
```
/git workflow complete feature
```

**Process:**
1. Check branch status
2. Run tests
3. Prepare for merge
4. Show PR creation instructions

### Clean Operations

#### Clean Working Directory
```
/git clean [dry-run]
```

**Operations:**
- Remove untracked files
- Clean ignored files
- Reset staged changes
- Show what would be deleted (dry-run)

### Status Operation

#### Enhanced Status
```
/git status [--verbose]
```

**Display:**
- Current branch and tracking
- Staged changes with diff
- Unstaged changes with diff
- Untracked files
- Stash list
- Recent commits

**Process:**
1. Run `git status`
2. Run `git branch --show-current`
3. Check for unpushed commits
4. Show remote tracking info
5. Display stashed changes if any

### Diff Operation

#### Show Changes
```
/git diff [file?] [--staged]
```

**Options:**
- No args: Show unstaged changes
- `--staged`: Show staged changes
- `file`: Show changes for specific file

**Process:**
1. Determine what to diff
2. Run appropriate git diff command
3. Format output for readability
4. Show file summary with line counts

### Log Operation

#### Commit History
```
/git log [--oneline] [-n] [--graph]
```

**Options:**
- `--oneline`: Compact format (default)
- `-n`: Number of commits (default: 10)
- `--graph`: Show branch graph
- `--all`: Show all branches

**Process:**
1. Run git log with options
2. Format output for readability
3. Show branch indicators
4. Include commit references

### Checkout Operation

#### Checkout Branch
```
/git checkout <branch-name>
```

**Process:**
1. Check for uncommitted changes
2. Warn if changes exist
3. Stash or commit if requested
4. Checkout target branch
5. Show status after checkout

#### Checkout File
```
/git checkout <file-path>
```

**Process:**
1. Confirm file restoration
2. Restore file from HEAD
3. Show what was restored

#### Create and Checkout
```
/git checkout -b <new-branch> [base-branch]
```

**Process:**
1. Ensure clean working directory
2. Checkout base branch if specified
3. Pull latest changes
4. Create and checkout new branch
5. Show branch info

## Best Practices

### Branch Management
1. **Keep branches focused**: One feature or bugfix per branch
2. **Short-lived branches**: Merge frequently to avoid conflicts
3. **Descriptive names**: Use clear, descriptive branch names
4. **Clean history**: Squash commits before merging when appropriate

### Commit Messages
1. **Use conventional commits**: Follow the type(scope): subject format
2. **Be descriptive**: Explain what and why, not how
3. **Keep subject short**: Limit to 50 characters
4. **Reference issues**: Link to related issue numbers

### Merge Practices
1. **Pull before merging**: Always get latest changes
2. **Review before merging**: Check diff and potential issues
3. **Test after merging**: Run tests to verify merge
4. **Resolve conflicts**: Use `/git-resolve` for conflict resolution assistance
5. **Communicate**: Notify team of significant changes

### Workflow Tips
1. **Feature branches**: Use for new features and enhancements
2. **Bugfix branches**: Use for bug fixes (can branch from release)
3. **Hotfix branches**: Use for urgent production fixes
4. **Protect main**: Require PRs for main branch changes

## Process Examples

### Feature Development Workflow
```bash
# 1. Start feature
/git workflow start feature add-oauth

# 2. Work on feature (coding happens here)

# 3. Commit changes
/git commit smart

# 4. Push to remote
git push origin feature/add-oauth

# 5. Create PR (manual or via tool)

# 6. After review and approval
/git workflow complete feature
```

### Bug Fix Workflow
```bash
# 1. Start bugfix
/git branch create bugfix/login-crash

# 2. Fix the bug

# 3. Commit fix (use /commit for proper commit handling)
/commit "bugfix: fix login crash on timeout"

# 4. Push and create PR
git push origin bugfix/login-crash

# 5. If merge conflicts occur during merge
/git-resolve interactive
```

### Hotfix Workflow
```bash
# 1. Create hotfix from main
/git branch create hotfix/security-patch

# 2. Fix security issue

# 3. Commit and push
/commit "hotfix: patch security vulnerability"
git push origin hotfix/security-patch

# 4. Merge to main (resolve conflicts if needed)
git checkout main
git merge hotfix/security-patch
# If conflicts: /git-resolve
```

## Dependencies

- **cache**: Read branch protection rules and workflow configs, update workflow state
- **doc**: Document Git workflow and conventions

## Completion Criteria

- [ ] Operation completed successfully
- [ ] Working directory is clean before branch operations
- [ ] Changes committed with descriptive messages (if committing)
- [ ] Merge completed without conflicts or conflicts resolved
- [ ] Remote branches synchronized (if pushing)
- [ ] Team notified of significant changes (if applicable)
- [ ] Documentation updated for workflow changes (if applicable)
- [ ] Git safety protocols followed
- [ ] User warned before destructive operations
- [ ] Hook failures handled appropriately (new commits)
