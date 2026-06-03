# Git Resolve Skill

The `/git-resolve` skill provides comprehensive assistance for resolving Git merge conflicts, rebase conflicts, and other Git conflict scenarios.

**Purpose**: Resolve merge conflicts with guided strategies, step-by-step instructions, and best practices

## Usage

```
/git-resolve [strategy]
```

**Strategies:**
- `interactive` - Interactive conflict resolution with step-by-step guidance (default)
- `manual` - Manual conflict resolution with detailed instructions
- `ours` - Accept our/HEAD changes for all conflicts
- `theirs` - Accept their/incoming changes for all conflicts
- `status` - Show current conflict status without resolving
- `abort` - Abort the current merge/rebase and return to pre-conflict state

**Examples:**
- `/git-resolve` - Interactive conflict resolution
- `/git-resolve status` - Show conflict status
- `/git-resolve manual` - Manual resolution guidance
- `/git-resolve ours` - Accept our changes globally
- `/git-resolve theirs` - Accept their changes globally
- `/git-resolve abort` - Abort current operation

## When to Use

- **During merge conflicts**: After `git merge` reports conflicts
- **During rebase conflicts**: When `git rebase` stops with conflicts
- **During cherry-pick conflicts**: When cherry-picking commits with conflicts
- **Before committing**: Must resolve all conflicts before finalizing
- **After failed auto-merge**: When Git cannot automatically resolve changes

## Conflict Detection

### Check for Conflicts

```bash
# Check git status for conflicts
git status

# Look for:
# - "both modified" files
# - "Unmerged paths" section
# - Conflict markers in files
```

### Conflict Markers

```
<<<<<<< HEAD
Your changes (current branch)
=======
Their changes (incoming branch)
>>>>>>> feature-branch
```

## Resolution Strategies

### 1. Interactive Resolution (Default)

**Process:**
1. List all conflicted files
2. For each conflicted file:
   - Show conflict markers and both versions
   - Display context around the conflict
   - Suggest resolution based on code analysis
   - Help choose or merge changes
3. Stage resolved files
4. Verify all conflicts resolved
5. Complete merge/rebase commit

**When to use:**
- Multiple conflicts to resolve
- Need context for each conflict
- Want to review each change carefully
- Complex merge scenarios

### 2. Manual Resolution

**Step-by-Step Process:**

1. **Detect Conflicts**
   ```bash
   git status
   ```

2. **Open Conflicted File**
   - Search for conflict markers: `<<<<<<<`, `=======`, `>>>>>>>`
   - Review both versions
   - Understand what each side changed

3. **Choose Resolution Strategy**
   - **Accept ours**: Keep your changes entirely
   - **Accept theirs**: Use their changes entirely
   - **Manual merge**: Combine both changes thoughtfully
   - **Custom edit**: Create new resolution

4. **Remove Conflict Markers**
   - Delete all conflict markers
   - Keep only the desired code
   - Ensure syntax is correct

5. **Validate Resolution**
   ```bash
   git diff --check              # Check for whitespace issues
   <test command>                # Run affected tests
   <build command>               # Build project
   ```

6. **Stage Resolved File**
   ```bash
   git add <resolved-file>
   ```

7. **Complete Operation**
   ```bash
   git commit                    # For merge conflicts
   git rebase --continue         # For rebase conflicts
   ```

**When to use:**
- Want full control over resolution
- Need to understand each change
- Complex business logic conflicts
- Learning conflict resolution

### 3. Accept Ours (Current Changes)

```bash
# Single file
git checkout --ours <file>
git add <file>

# All conflicts
git checkout --ours .
git add .

# For specific file types
find . -name '*.ts' -exec git checkout --ours {} \;
git add .
```

**When to use:**
- Your changes are correct
- Incoming changes are outdated
- You know your version should win
- Quick resolution needed

### 4. Accept Theirs (Incoming Changes)

```bash
# Single file
git checkout --theirs <file>
git add <file>

# All conflicts
git checkout --theirs .
git add .

# For specific file types
find . -name '*.ts' -exec git checkout --theirs {} \;
git add .
```

**When to use:**
- Incoming changes are correct
- Your changes should be overwritten
- Trust upstream changes
- Syncing with main branch

### 5. Use Merge Tool

```bash
# Configure merge tool in ~/.gitconfig:
[merge]
  tool = vscode
[mergetool "vscode"]
  cmd = code --wait $MERGED

# Use the tool
git mergetool

# For specific file
git mergetool <file>
```

**Popular tools:**
- **VSCode**: `code --wait $MERGED`
- **Vim**: `vimdiff`
- **Emacs**: `emacs`
- **Kdiff3**: Visual diff/merge tool
- **Beyond Compare**: Commercial tool

**When to use:**
- Visual comparison helps
- Complex line-by-line conflicts
- Prefer GUI over terminal

### 6. Abort Operation

```bash
# Abort merge
git merge --abort

# Abort rebase
git rebase --abort

# Abort cherry-pick
git cherry-pick --abort
```

**When to use:**
- Too many conflicts
- Need to reconsider approach
- Want to start over
- Merge was a mistake

## Conflict Status

### Check Status

```bash
/git-resolve status
```

**Output:**
- List of unmerged files
- Conflict count per file
- Merge/rebase state
- Next steps suggestion
- Affected branches

**Example Output:**
```
━━━ Conflict Status ━━━

Operation: Merge (feature/auth → main)
State: 3 files have conflicts

Conflicted Files:
  ✗ src/auth/login.ts (2 conflicts)
  ✗ src/auth/service.ts (1 conflict)
  ✗ tests/auth.test.ts (1 conflict)

Suggested Action: /git-resolve interactive
```

## Common Conflict Scenarios

### Same Line Changes

Both branches modified the same lines of code.

**Resolution:**
- Manually review both versions
- Determine which changes are needed
- May need to combine both
- Consult with original author if unsure

**Example:**
```javascript
// HEAD
const user = await User.findById(id);

// feature-branch
const user = await User.findByEmail(email);

// Resolution: Keep both if needed
const user = await User.findById(id) || await User.findByEmail(email);
```

### Rename Conflicts

File was renamed differently in each branch.

**Detection:**
```bash
git status
# Shows: "renamed: oldfile.js -> newfileA.js"
# Shows: "renamed: oldfile.js -> newfileB.js"
```

**Resolution:**
- Choose appropriate name
- Git may handle automatically
- Use `git status` for details
- Consider using one name consistently

### Binary File Conflicts

Images, PDFs, compiled files, etc.

**Resolution:**
```bash
# Choose version
git checkout --ours <binary-file>
# OR
git checkout --theirs <binary-file>

git add <binary-file>
```

**Prevention:**
- Add binary files to `.gitattributes`
- Use LFS for large binaries
- Document binary file handling

### Dependency Conflicts

Package manager files (package.json, go.mod, etc.).

**Resolution:**
- Use `package-lock.json` merge driver (if available)
- Manually merge version requirements
- Run `npm install` / `go mod tidy` after resolution
- Test build and run tests

### Whitespace Conflicts

Only whitespace differs between versions.

**Detection:**
```bash
git diff -w    # Ignore whitespace
```

**Resolution:**
```bash
# Accept one version
git checkout --ours <file>
git add <file>
```

**Prevention:**
```bash
# .gitattributes
*.text text eol=lf
*.js text eol=lf
```

## Post-Resolution Steps

### 1. Verify Resolution

```bash
# Check for remaining conflicts
git status

# Check for whitespace issues
git diff --check

# Review all changes
git diff
```

### 2. Test Changes

```bash
# Run affected tests
npm test -- <affected-tests>
go test ./<affected-package>

# Build project
npm run build
go build

# Manual testing if needed
```

### 3. Complete Operation

**For Merge:**
```bash
git add <resolved-files>
git commit    # May be automatic for fast-forward
```

**For Rebase:**
```bash
git add <resolved-files>
git rebase --continue
```

**For Cherry-Pick:**
```bash
git add <resolved-files>
git cherry-pick --continue
```

### 4. Cleanup (Optional)

```bash
# Remove conflict marker files
rm .git/*.orig

# Update remote if needed
git push origin <branch>
```

## Conflict Prevention

### Before Starting Work

```bash
# Pull latest changes
git pull origin main

# Keep branches up to date
git fetch --all
```

### During Development

- **Pull frequently** to reduce divergence
- **Keep branches short-lived** to minimize conflict opportunities
- **Communicate with team** about overlapping work
- **Use feature flags** instead of merge conflicts when possible

### Before Merging

```bash
# Check for potential conflicts
git merge-base --is-ancestor main HEAD

# Preview merge
git merge --no-commit --no-ff main
git merge --abort    # If just previewing
```

### Team Practices

- **Branch naming**: Use clear branch names (feature/, bugfix/)
- **PR reviews**: Review PRs early to catch potential conflicts
- **Trunk-based development**: Short-lived branches reduce conflicts
- **Code owners**: Assign owners for critical files

## Best Practices

### Resolution Strategy

1. **Understand the conflict**: Read both versions carefully
2. **Communicate**: Talk to the other developer if unsure
3. **Test**: Always test after resolving conflicts
4. **Document**: Add comments if resolution is complex
5. **Learn**: Understand why the conflict occurred

### Safety

- **Never blindly accept**: Always review conflicts
- **Test after resolution**: Run tests and build
- **Keep history**: Don't force push to avoid conflicts
- **Ask for help**: Complex conflicts may need team input

### Workflow Integration

```bash
# Typical workflow with conflicts
git checkout -b feature/new-auth
# ... make changes ...
git commit -am "feat: add new auth"

# Try to merge to main
git checkout main
git pull origin main
git merge feature/new-auth
# CONFLICT!

# Resolve conflicts
/git-resolve interactive

# Test
npm test

# Complete
git push origin main
```

## Process Examples

### Merge Conflict Workflow

```bash
# 1. Start merge
git merge feature/user-profile
# Auto-merge failed; fix conflicts and then commit the result.

# 2. Check status
/git-resolve status
# Shows: 2 files with conflicts

# 3. Interactive resolution
/git-resolve
# Walks through each conflict:
# - src/user/profile.ts: Accept ours for line 45, merge for line 78
# - tests/user/profile.test.ts: Accept theirs

# 4. Verify
git status
git diff --check

# 5. Test
npm test

# 6. Complete
git commit
```

### Rebase Conflict Workflow

```bash
# 1. Start rebase
git rebase main
# Could not apply 3a2f1c1... feat: add auth

# 2. Check status
/git-resolve status

# 3. Resolve conflicts
/git-resolve manual

# 4. Continue rebase
git add .
git rebase --continue

# 5. If more conflicts, repeat from step 2

# 6. Final push
git push --force-with-lease origin feature/auth
```

### Multiple Branches Conflict

```bash
# When merging into main with multiple feature branches
git checkout main
git merge feature-auth
# CONFLICT in auth.ts

# Resolve first conflict
/git-resolve
git commit

# Merge second branch
git merge feature-ui
# CONFLICT in layout.tsx

# Resolve second conflict
/git-resolve
git commit
```

## Advanced Scenarios

### Recursive Merge Strategy

```bash
# Use recursive merge with patience
git merge -s recursive -X patience feature-branch

# Prefer theirs for specific files
git merge -s recursive -X theirs=src/config.ts feature-branch

# Ignore whitespace
git merge -X ignore-space-at-eol feature-branch
```

### Octopus Merge

```bash
# Merge multiple branches at once
git merge feature-auth feature-ui feature-api
# Resolve conflicts that involve all branches
```

### Submodule Conflicts

```bash
# Update submodule
git submodule update --remote

# Resolve submodule conflict
cd <submodule>
git checkout main
cd ..
git add <submodule>
```

## Completion Criteria

- [ ] All conflicts identified and listed
- [ ] Each conflict resolved with chosen strategy
- [ ] Conflict markers removed from all files
- [ ] `git status` shows no conflicts
- [ ] `git diff --check` passes
- [ ] Affected tests passing
- [ ] Build successful
- [ ] Merge/rebase commit completed
- [ ] Changes pushed to remote (if applicable)
- [ ] Team notified if significant conflicts occurred

## Related Skills

- `/git` - General git operations (status, branch, merge)
- `/commit` - Commit changes after resolving conflicts
- `/pr` - Pull request operations after resolving conflicts

## Dependencies

- **git**: Git must be installed and available
- **editor**: Text editor for manual resolution (optional)

## Version

**Created**: 2026-03-12
**Purpose**: Extracted from /git skill for focused conflict resolution assistance
