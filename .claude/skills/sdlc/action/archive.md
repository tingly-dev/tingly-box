# Archive Skill

The `/archive` skill manages SDLC documentation by moving older documents to the `.sdlc/docs/archive/` directory while preserving directory structure and metadata.

## Usage

```
/archive [scope] [pattern?] [reason?]
```

**Scopes:**
- `spec` - Archive specification documents
- `research` - Archive research documents
- `pencil` - Archive pencil/design sketch documents
- `understand` - Archive understanding documents
- `arch` - Archive architecture documents
- `all` - Archive all SDLC-related documents

**Pattern (optional):**
- Glob pattern to match specific files (e.g., `"20260308*"`, `"sdlc-v2*"`)
- **If omitted**: Auto-select documents from last month and older

**Reason (optional):**
- Why these documents are being archived
- Will be recorded in metadata

**Examples:**
```bash
/archive spec "20260308*" "Replaced by v4 spec"
/archive research                # Archives last month+ docs
/archive pencil "sdlc-v2*" "Outdated design drafts"
/archive all "Completed user auth feature"
```

## Directory Structure

```
docs/
├── spec/          # Active specs
├── research/      # Active research
├── pencil/        # Active sketches
├── understand/    # Active understanding docs
├── arch/          # Active architecture docs
└── archive/       # Archived documents
    ├── spec/
    ├── research/
    ├── pencil/
    ├── understand/
    └── arch/
```

## Auto-Archive by Month

When no pattern is specified, automatically archives documents from **last month and older**:

```bash
# Current month: 2026-03
/archive research
# Archives: 2026-02 and older research docs

# Keeps: 2026-03 research docs active
```

**File pattern matching:**
- Spec files: `YYYYMMDD-*` → extracts date for comparison
- Research files: `YYYYMMDD-*` → extracts date for comparison
- Pencil files: `YYYY-MM-DD-*` → extracts date for comparison
- Other files: Uses file modification time

## Process

### 1. Document Selection

**With pattern:**
1. Parse scope to determine source directory
2. Use glob to match files against pattern
3. Show selection to user for confirmation

**Without pattern (auto-archive):**
1. Get current date
2. Calculate "last month" threshold
3. Scan scope directory for files
4. Extract dates from filenames or mtime
5. Select files at or before threshold
6. Show selection to user for confirmation

### 2. Metadata Preparation

For each file to archive:
1. Read original file content
2. Extract or generate metadata:
   - `archived_at`: Current timestamp
   - `archived_reason`: User-provided reason
   - `original_location`: Original file path
   - `original_created_at`: File creation date (extracted from filename)

### 3. File Migration

1. Create corresponding directory in `.sdlc/docs/archive/{scope}/`
2. Prepend metadata block to file content
3. Write to archived location
4. Remove original file
5. Track in archive log

### 4. Archive Log

Update `.sdlc/docs/archive/archive-log.json`:

```json
{
  "archives": [
    {
      "id": "arc-20260309-001",
      "archived_at": "2026-03-09T10:00:00Z",
      "scope": "spec",
      "pattern": "auto:2026-02",
      "reason": "Monthly cleanup",
      "files": [
        {
          "original": ".sdlc/docs/spec/20260228-old-spec.md",
          "archived": ".sdlc/docs/archive/spec/20260228-old-spec.md"
        }
      ]
    }
  ]
}
```

## Metadata Format

Each archived file gets a YAML frontmatter block:

```markdown
---
archived: true
archived_at: 2026-03-09T10:00:00Z
archived_reason: Monthly cleanup
original_location: .sdlc/docs/spec/20260228-old-feature.md
original_created_at: 2026-02-28
archived_by: /archive
---

# [Original Title]
[Original content continues...]
```

## Interactive Workflow

```bash
$ /archive research

Auto-archive: documents from 2026-02 and older

Found 3 files to archive in .sdlc/docs/research/:
  ✓ 20260223-tingly-spec-extension.md
  ✓ 20260115-oauth-research.md
  ✓ 20260110-api-design.md

Reason: Monthly cleanup (auto)

Proceed? (y/n) y

✓ Archived 20260223-tingly-spec-extension.md → .sdlc/docs/archive/research/
✓ Archived 20260115-oauth-research.md → .sdlc/docs/archive/research/
✓ Archived 20260110-api-design.md → .sdlc/docs/archive/research/

Updated archive log: .sdlc/docs/archive/archive-log.json
```

## Date Extraction Patterns

| File Pattern | Date Source | Example |
|-------------|-------------|---------|
| `YYYYMMDD-name.md` | Filename prefix | `20260228-feature.md` → 2026-02-28 |
| `YYYY-MM-DD-name.md` | Filename prefix | `2026-02-28-sketch.md` → 2026-02-28 |
| `name-YYYYMMDD.md` | Filename suffix | `feature-20260228.md` → 2026-02-28 |
| Other | File modification time | From `mtime` |

## Best Practices

### When to Archive

- **Monthly cleanup**: Use `/archive {scope}` to clean up last month's docs
- **After completing features**: Archive related spec/research docs
- **Design iterations**: Archive old pencil sketches when implementing new ones
- **Quarterly review**: Archive all documents older than 3 months

### When NOT to Archive

- Active work in progress
- Reference documentation still being used
- Documents that need version control history
- Documents from current month (unless explicitly patterned)

### Naming Conventions

- Use clear patterns for manual archiving
- Include date ranges for batch archiving
- Consider keeping recent iterations active

## Error Handling

- **No files found**: Inform user and suggest patterns
- **Directory doesn't exist**: Create archive directory structure
- **File already archived**: Skip with warning
- **Metadata conflict**: Append to existing metadata
- **Move failure**: Rollback and report error

## Completion Criteria

- [ ] Files moved maintaining directory structure
- [ ] Metadata added to each archived file
- [ ] Archive log updated
- [ ] Original files removed
- [ ] User confirmation obtained before destructive action
- [ ] Clear feedback on what was archived

## Dependencies

- **glob**: Find files matching patterns
- **fs**: File system operations
- **git**: Check if files are tracked (optional warning)

## Examples

### Example 1: Monthly Auto-Archive
```bash
# Current date: 2026-03-09
/archive research

# Archives all research docs from 2026-02 and older
# Keeps 2026-03 docs active
```

### Example 2: Archive Old Versions
```bash
/archive spec "v1*" "Replaced by v2 architecture"

# Moves:
# .sdlc/docs/spec/v1-user-auth.md → .sdlc/docs/archive/spec/v1-user-auth.md
# .sdlc/docs/spec/v1-api-design.md → .sdlc/docs/archive/spec/v1-api-design.md
```

### Example 3: Archive Completed Work
```bash
/archive all "user-auth-feature" "Feature completed and merged"

# Moves matching files from all scopes to archive/
```

### Example 4: Archive Old Design Iterations
```bash
/archive pencil "sdlc-v2*" "Replaced by v3 design"

# Moves:
# .sdlc/docs/pencil/2026-03-08-sdlc-v2-simplified.md → .sdlc/docs/archive/pencil/2026-03-08-sdlc-v2-simplified.md
```

## Integration with SDLC

### End of Workflow
When completing a workflow:
```bash
/sdlc end
/archive all "Completed $workflow_type workflow"
```

### Monthly Cleanup
Automate monthly cleanup:
```bash
# At the start of each month
/archive spec
/archive research
/archive pencil
```

### Phase Transition
After major phase transitions:
```bash
/sdlc verify
/archive spec "old-*" "Spec verified and implemented"
```

## Restoration

To restore an archived document:
```bash
# Copy from archive back to active
# Remove archive metadata
# Update archive log with restoration entry
```

## Output Format

```
═══ Archive Documents ═══

Scope:   spec
Pattern: 202602* (auto: last month)
Reason:  Monthly cleanup

Found 4 files:
  ✓ 20260228-sdlc-skills-v3.md
  ✓ 20260215-old-feature.md
  ✓ 20260210-api-design-v1.md
  ✓ 20260205-research-notes.md

─────────────────────────────
Destination: .sdlc/docs/archive/spec/
Proceed? (y/n)
```

## Notes

- Archived files remain in git history
- Use `/archive` proactively to keep docs/ clean
- Auto-archive keeps current month's docs active
- The archive log provides audit trail
- "Archive" is more neutral than "deprecate" - documents still have value
