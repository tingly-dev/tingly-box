# Feedback Skill

Collect user feedback on skill execution quality, gather improvement suggestions through interactive questions, and update skills based on feedback.

## Usage

```bash
/feedback [skill_name]
```

**Examples:**
```bash
/feedback discuss              # Feedback on discuss skill
/feedback workflow:feature    # Feedback on feature workflow
/feedback action:coding       # Feedback on coding action
```

## Purpose

- **Continuous Improvement**: Systematically improve skill quality based on real usage
- **User-Driven Enhancement**: Capture user preferences and pain points
- **Context-Aware Updates**: Use conversation context to understand what worked/didn't work

## Process

### 1. Context Collection

First, gather execution context:
- **Current conversation**: What task was just completed?
- **Skill executed**: Which skill or workflow was used?
- **Output files**: What documents were created? (Check `.sdlc/docs/`, `.sdlc/arch/`, etc.)
- **User interactions**: How many questions were asked? Were they helpful?

### 2. Interactive Feedback Collection

Use `AskUserQuestion` tool to collect structured feedback across multiple dimensions:

#### Question 1: Overall Satisfaction
```
How satisfied are you with the [skill_name] execution?
Options:
- Excellent - Exceeded expectations
- Good - Met expectations
- Okay - Acceptable but needs improvement
- Poor - Did not meet expectations
```

#### Question 2: Specific Strengths
```
What aspects worked well? (multi-select)
Options:
- Clear and actionable output
- Appropriate level of detail
- Good use of context
- Helpful interactive questions
- Efficient execution
- Other [free text]
```

#### Question 3: Areas for Improvement
```
What could be improved? (multi-select)
Options:
- Too verbose / too brief
- Missed important context
- Asked too many questions
- Output format unclear
- Execution was slow
- Other [free text]
```

#### Question 4: Specific Suggestions
```
Any specific suggestions for improvement?
[Free text input]
```

### 3. Feedback Analysis

After collecting responses:

1. **Summarize feedback**
   - Overall satisfaction score
   - Key strengths identified
   - Main improvement areas
   - Specific user suggestions

2. **Review skill file**
   - Read current skill implementation
   - Identify sections that need updates
   - Check if similar feedback exists in version history

3. **Propose updates**
   - Present specific changes to the skill file
   - Explain how changes address the feedback
   - Ask user to confirm before applying

### 4. Skill Update

**IMPORTANT: Use user home feedback directory**
- **User backup dir**: `~/.sdlc.feedback/backup/`
- **User changelog dir**: `~/.sdlc.feedback/changelog/`
- **User feedback docs**: `~/.sdlc.feedback/docs/`

**CRITICAL: Only update global commands in `~/.claude/commands/`**

This skill ONLY edits global commands located at:
- **Global skills**: `~/.claude/commands/*.md`
- **DO NOT edit** project-local skills (`action/`, `workflow/`)

If user approves updates:

1. **Verify skill location**
   - Check if skill exists at `~/.claude/commands/[skillname].md`
   - If NOT in global commands → inform user, provide feedback only (no auto-edit)
   - Always display **absolute paths** to avoid confusion

2. **Backup current version**: Copy to user feedback directory
   ```bash
   # Backup to: ~/.sdlc.feedback/backup/
   mkdir -p ~/.sdlc.feedback/backup
   cp ~/.claude/commands/feedback.md ~/.sdlc.feedback/backup/feedback-v1.0.0-20260326.md.bak
   ```

3. **Update skill file**: Use `Edit` tool on global command
   ```
   Edit ~/.claude/commands/feedback.md  # ✅ Absolute path
   ```

4. **Increment version**: Update version at bottom of skill file
   - Example: `v1.0.0` → `v1.1.0`

5. **Create changelog**: Write to user feedback directory
   ```
   ~/.sdlc.feedback/changelog/feedback-v1.1.0-20260326.changelog.md
   ```

**Example Paths (Always Absolute):**
```
Source:     /Users/username/.claude/commands/feedback.md
Backup:     ~/.sdlc.feedback/backup/feedback-v1.0.0-20260326.md.bak
Changelog:  ~/.sdlc.feedback/changelog/feedback-v1.1.0-20260326.changelog.md
```

**Recovery:**
```bash
# Restore from backup
cp ~/.sdlc.feedback/backup/feedback-v1.0.0-20260326.md.bak ~/.claude/commands/feedback.md
```

## Output Structure

### Feedback Session Document

Save to: `~/.sdlc.feedback/docs/feedback-[skill-name]-[date].feedback.md`

**Format:**
```markdown
# Feedback: [Skill Name]

**Date**: 2026-03-26
**Skill Location**: /Users/username/.claude/commands/feedback.md
**Session Context**: [Brief 1-line description]

## User Feedback
- **Satisfaction**: Good
- **Strengths**: Clear output, good detail
- **Needs Work**: Too many questions
- **Suggestions**: Show code earlier

## Proposed Changes
1. **Add code previews**: Show snippets in options
2. **Reduce questions**: From 2 to 1 follow-up

## Status
- [x] User approved
- [x] Backup: `~/.sdlc.feedback/backup/feedback-v1.0.0-20260326.md.bak`
- [x] Updated: `/Users/username/.claude/commands/feedback.md` v1.0.0 → v1.1.0
- [x] Changelog: `~/.sdlc.feedback/changelog/feedback-v1.1.0-20260326.changelog.md`
```

### Changelog Document

Save to: `~/.sdlc.feedback/changelog/[skill-name]-[version]-[date].changelog.md`

**Format:**
```markdown
# Changelog: [skill-name] v[old] → v[new]

**Date**: 2026-03-26
**Skill**: `[skill-reference]`

## User Feedback Summary
[1-2 sentences max]

## Changes Applied
1. [Change 1]
2. [Change 2]

## Impact
[Expected improvement]
```

**Keep changelogs concise** - max 10 lines each.

## Key Principles

1. **Timing**: Run feedback immediately after skill execution while context is fresh
2. **Context-Aware**: Reference specific outputs, questions asked, and results produced
3. **Actionable**: Focus feedback on concrete, implementable improvements
4. **Iterative**: Track feedback over time to identify patterns
5. **Transparent**: Show user exactly what will change before applying updates

## Example Flow

**User runs**: `/discuss State management`
**After completion**: `/feedback discuss`

```markdown
I'll help collect feedback on the `discuss` skill.

## Context
- **Skill**: /discuss (global command)
- **Location**: /Users/username/.claude/commands/discuss.md
- Topic: State management for React
- Presented 3 approaches
- Asked 1 clarifying question
- User selected Zustand

[Shows AskUserQuestion with 4 structured questions]

---

## Feedback Summary

✅ **Satisfaction**: Good

**Strengths**: Clear output, appropriate detail
**Needs Work**: Too many questions
**Suggestion**: "Show code examples earlier"

## Proposed Changes

1. **Add code previews in options**
   - Shows implementation style upfront

2. **Reduce follow-up questions**
   - Present → choose → details (skip "needs" question)

Apply these changes to `/Users/username/.claude/commands/discuss.md`?
```

**If approved**:
- Backs up to `~/.sdlc.feedback/backup/discuss-v1.0.0-20260326.md.bak`
- Updates `/Users/username/.claude/commands/discuss.md` → v1.1.0
- Creates `~/.sdlc.feedback/changelog/discuss-v1.1.0-20260326.changelog.md`
- Saves `~/.sdlc.feedback/docs/feedback-discuss-20260326.feedback.md`

## When to Use

- ✅ After executing any **global command** (`/feedback`, `/discuss`, `/commit`, etc.)
- ✅ When user explicitly requests feedback
- ✅ Periodically for frequently-used skills
- ❌ Not for project-local skills (`action/`, `workflow/`)
- ❌ Not during workflow execution (wait until complete)
- ❌ Not for simple utility commands (git, cache)

## Scope Limitation

**This skill ONLY edits global commands:**
- ✅ Files in `~/.claude/commands/*.md`
- ❌ Project files in `action/`, `workflow/`

**Why?** Project-local skills are version-controlled and team-shared. Global commands are user-specific configurations.

## Dependencies

- `Read`: Review skill files and output documents
- `Edit`: Update skill files based on feedback
- `AskUserQuestion`: Collect structured feedback
- `action:doc`: Optional - log feedback history

## Success Metrics

Good feedback collection should:
- Complete in < 2 minutes
- Ask 3-5 focused questions
- Result in 1-3 concrete, actionable changes
- Improve user satisfaction over time

---

**Version**: 1.3.0 | **Updated**: 2026-05-13
