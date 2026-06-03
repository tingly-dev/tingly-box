# /sdlc

Software Development Lifecycle management with intelligent intent detection and routing.

## Usage

```bash
/sdlc [natural language request]  # Smart mode - AI detects intent
/sdlc [command] [args]             # Explicit command
```

---

# Routing Tables

> For AI execution: match input against Cmd (exact) first, then Intent keywords.
> Always show: `🎯 Detected: <intent>  → Executing: <skill>`

## Actions

| Skill             | Cmd        | Intent keywords                                 |
| ----------------- | ---------- | ----------------------------------------------- |
| action:guard      | guard      | safety, before work                             |
| action:plan       | plan       | plan, design plan, 规划                         |
| action:understand | understand | understand, analyze architecture, build context |
| action:cr         | cr         | review, check, audit, find issues, 检查         |
| action:spec       | spec       | spec, specification, write spec, 规范           |
| action:test       | test       | test, run tests, 测试                           |
| action:commit     | commit     | commit, save changes, 提交                      |
| action:pr         | pr         | pull request, 提交pr                            |
| action:debug      | debug      | debug, diagnose                                 |
| action:lint       | lint       | lint, fix style, check style                    |
| action:simplify   | simplify   | simplify, clean up code, 简化                   |
| action:regression | regression | regression, check regressions                   |
| action:research   | research   | research, investigate, compare, 研究            |
| action:discuss    | discuss    | discuss, talk about                             |
| action:handoff    | handoff    | delegate, handoff                               |
| action:secure     | secure     | security, secure                                |
| action:harness    | harness    | harness, verification                           |
| action:validate   | validate   | validate                                        |
| feedback          | feedback   | feedback, score                                 |

## workflow

| Skill             | Intent keywords                           | Pipeline                                                          |
| ----------------- | ----------------------------------------- | ----------------------------------------------------------------- |
| workflow:bugfix   | fix, bug, issue, error, 修复              | understand→debug→coding→test→validate→secure→commit→pr            |
| workflow:feature  | add, new feature, implement, 添加, 新功能 | understand→research→spec→coding→test→validate→secure→cr→commit→pr |
| workflow:refactor | refactor, clean up, 重构                  | understand→spec→coding→test→commit→pr                             |
| workflow:research | research, investigate, 研究               | understand→research→doc→discuss→END                               |
| workflow:minor    | minor, small change, 小改动               | coding→test→commit                                                |


---

# Key Behaviors

- `explore/explain/how does` → read and explain inline, no skill invoked
- `understand/analyze architecture` → `action:understand` (creates `.sdlc/arch/` cache)
- `review/check/find issues` → `action:cr` (creates `*.cr.md`)

## Output Structure

```
.sdlc/
├── docs/      # category-feature-date.type.md
├── harness/   # verification harnesses
└── arch/      # architecture cache
```

**IMPORTANT:** `.sdlc` folder should be placed under the user's coding project path.
