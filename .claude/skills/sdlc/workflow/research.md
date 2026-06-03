# Research Workflow

**Purpose**: Explore topics and document findings without implementation.

> **Critical**: Always start with `/sdlc understand` to understand the current codebase before researching external solutions. This ensures your research considers existing architecture and constraints.

## When to Use

Use this workflow when:
- Investigating new technologies
- Exploring architectural options
- Analyzing competitors
- Documenting domain knowledge
- Preparing for future features
- Creating technical proposals

## Workflow Sequence

```
START
  │
  ▼
understand → research → doc → discuss → END
```

**First Step is Non-Negotiable:**
1. `understand` - Know what you currently have before researching alternatives

## Phase Details

### 0. Understand (Required First Step)
```bash
/sdlc understand [scope]
```
- Understand current architecture and implementation
- Identify constraints and integration points
- Review existing patterns and conventions
- **Research should consider what you already have**

### 1. Research
```bash
/sdlc research "Topic or question to investigate"
```

**Activities:**
- Literature review
- Technology evaluation
- Prototype/experiment creation
- Data collection
- Analysis and synthesis
- Risk assessment

**Deliverables:**
- Research findings
- Comparison tables
- Proof of concepts
- Recommendations

### 2. Doc
```bash
/sdlc doc "Create research documentation"
```

**Document Contents:**
- Executive summary
- Background and context
- Methodology
- Findings and analysis
- Comparison matrices
- Recommendations
- Next steps
- References and sources

**Documentation Types:**
- ADR (Architecture Decision Record)
- Technical proposal
- Research report
- Comparison analysis
- Proof of concept summary

### 3. Discuss
```bash
/sdlc discuss "Present findings for feedback"
```

**Activities:**
- Team presentation
- Stakeholder review
- Feedback collection
- Q&A sessions
- Decision making
- Action item tracking

**Outcomes:**
- Consensus on approach
- Approved direction
- Follow-up tasks
- Transition to feature workflow (if approved)

## Usage Example

```bash
# Start research workflow
/sdlc start research "Evaluate real-time collaboration solutions"

# Step 1: Understand current architecture (MANDATORY)
/sdlc understand
# → Learn about current data flow, state management, networking layer

# Step 2: Research phase
/sdlc research "Compare Yjs, Automerge, and custom CRDT implementation"
# → Creates comparison table, performance benchmarks, POC code

# Step 3: Document findings
/sdlc doc "Create technical proposal with recommendation"
# → Generates ADR document with pros/cons and final recommendation

# Step 4: Discuss with team
/sdlc discuss "Present findings to engineering team"
# → Get feedback, make decision, plan next steps

# [END - or transition to feature workflow if implementing]
```

## Anti-Pattern: What NOT to Do

```bash
# ❌ BAD: Research without understanding current system
/sdlc research "Add real-time collaboration"
# → Proposes solutions that don't fit existing architecture
# → Misses integration challenges
# → Wastes time researching incompatible options

# ✅ GOOD: Understand first, then research
/sdlc understand     # Know what you have
/sdlc research       # Research solutions that fit
# → Faster research, better recommendations, smoother implementation
```

## Research Output Template

```markdown
# Research: [Topic]

## Summary
[Brief overview of findings and recommendation]

## Background
[Context and motivation for this research]

## Options Evaluated
### Option 1: [Name]
- **Pros**: ...
- **Cons**: ...
- **Use cases**: ...

### Option 2: [Name]
- **Pros**: ...
- **Cons**: ...
- **Use cases**: ...

## Comparison Matrix
| Feature     | Option 1 | Option 2 | Option 3 |
| ----------- | -------- | -------- | -------- |
| Performance | ...      | ...      | ...      |
| Complexity  | ...      | ...      | ...      |
| Cost        | ...      | ...      | ...      |

## Recommendation
**Selected**: [Option X]

**Reasoning**:
- ...
- ...

## Implementation Considerations
- Effort estimate: ...
- Risks: ...
- Dependencies: ...

## Next Steps
1. [If approved] Transition to feature workflow
2. [If rejected] Document reasoning and archive
3. [If deferred] Schedule review date

## References
- [Link 1]
- [Link 2]
```

## Completion Checklist

- [ ] **Understand phase completed** (know current architecture)
- [ ] Research questions defined
- [ ] Investigation completed
- [ ] Findings documented
- [ ] Recommendations made
- [ ] Stakeholders consulted
- [ ] Decision recorded
- [ ] Next steps identified

## Transition to Implementation

If research leads to implementation decision:

```bash
# After research completes and is approved
/sdlc start feature "Implement approved solution"
# → Transitions to feature workflow with research as foundation
```

## Research Types

### 1. Technology Evaluation
Compare frameworks, libraries, tools
**Output**: Recommendation with justification

### 2. Architecture Exploration
Design system architecture, patterns
**Output**: Architecture diagrams, ADR

### 3. Proof of Concept
Build prototype to validate approach
**Output**: Working POC, feasibility report

### 4. Domain Research
Understand problem domain, requirements
**Output**: Domain model, user stories

### 5. Competitive Analysis
Study competing solutions
**Output**: Feature comparison, gap analysis

## Key Differences from Other workflow

| Feature        | Bugfix         | Refactor       | Research           |
| -------------- | -------------- | -------------- | ------------------ |
| Produces code  | Produces fix   | Improves code  | Produces knowledge |
| Has test phase | Has test phase | Has test phase | No test phase      |
| Merges to main | Merges to main | Merges to main | Ends with doc      |
| User-facing    | Bug-focused    | Internal       | Informative        |

## Notes

- Research workflow **does not produce code**
- No testing, security, or PR phases
- Output is documentation and knowledge
- Use `/pencil` for diagrams and visualizations
- Use `/cache` to store research data
- Can transition to feature workflow if implementation needed
- Always document sources and references
- Include both pros and cons for balanced analysis
