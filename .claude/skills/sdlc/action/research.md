# Research Phase Skill

Researches technical topics, analyzes options, and documents findings to support informed decision-making.

## Usage

```
/sdlc research [topic]
```

## Description

The research phase skill conducts thorough technical research on specified topics. It gathers information from multiple sources, analyzes different approaches, compares options, and documents findings with clear recommendations.

### When to Use

- Exploring new technologies or frameworks
- Investigating solutions for technical challenges
- Comparing implementation approaches
- Understanding best practices
- Evaluating libraries, tools, or services
- Researching architectural patterns

## Process

1. **Understand Research Goals**
   - Clarify the research question or problem
   - Identify constraints and requirements
   - Determine depth and scope needed

2. **Information Gathering**
   - Search relevant documentation and resources
   - Review official sources and best practices
   - Examine real-world examples and case studies
   - Consult community knowledge and discussions

3. **Analysis and Evaluation**
   - Compare multiple approaches/options
   - Identify pros and cons of each
   - Consider trade-offs (performance, complexity, maintainability)
   - Assess fit for specific use case

4. **Documentation**
   - Summarize key findings
   - Provide clear recommendations with rationale
   - Include code examples or references where helpful
   - Note any caveats or areas requiring further investigation

## Output Format

The research skill produces a structured research summary with:

### Research Summary

**Topic:** [Research Topic]

**Research Question:** [What was being investigated]

**Key Findings:**
- [Major finding 1]
- [Major finding 2]
- [Major finding 3]

**Options Analyzed:**

**Option 1: [Name]**
- Description: [Brief description]
- Pros: [Advantages]
- Cons: [Disadvantages]
- Best for: [Use cases]
- Example: [Code or reference if applicable]

**Option 2: [Name]**
- Description: [Brief description]
- Pros: [Advantages]
- Cons: [Disadvantages]
- Best for: [Use cases]
- Example: [Code or reference if applicable]

**Recommendation:**
[Recommended approach with clear rationale, including specific reasons why this option is best for the use case]

**Additional Resources:**
- [Resource 1 - URL or reference]
- [Resource 2 - URL or reference]

**Areas for Further Investigation:**
- [Topic 1]
- [Topic 2]

## Completion Checklist

- [ ] Research question clearly understood
- [ ] Multiple sources consulted
- [ ] Multiple options analyzed (if applicable)
- [ ] Pros and cons documented
- [ ] Clear recommendation provided with rationale
- [ ] Examples or references included where helpful
- [ ] Additional needs identified
- [ ] Documented using doc.md skill

## Examples

### Example 1: Technology Research

```
/sdlc research state management options for Next.js 14
```

Would produce a comparison of:
- React Context + hooks
- Zustand
- Redux Toolkit
- Jotai/Recoil
- Server state vs client state considerations

### Example 2: Implementation Approach

```
/sdlc research authentication strategies for API routes
```

Would analyze:
- JWT vs session-based
- NextAuth.js
- Supabase Auth
- Clerk
- Custom implementation considerations

### Example 3: Best Practices

```
/sdlc research error handling patterns in TypeScript
```

Would investigate:
- Try-catch patterns
- Result types (neverthrow effects)
- Zod for validation
- Global error handlers
- Logging and monitoring strategies

## Integration

This skill is typically invoked as the first phase in the SDLC workflow:

1. **Research Phase** (this skill) - Gather information and options
2. **Spec Phase** (/sdlc spec) - Create detailed specification based on research
3. **Coding Phase** (/sdlc coding) - Implement based on spec

The research phase helps ensure that specifications are grounded in thorough analysis and that the best approach is chosen before committing to implementation.

## Related Skills

- **doc.md** - Used to document research findings
- **pencil.md** - May be used for creating diagrams in research documentation
- **spec.md** - Next phase: creates specification based on research

## Best Practices

- Be specific in your research topic for better results
- Mention any specific constraints (e.g., "for a small team" or "must be open source")
- Research can be iterative - follow-up with deeper research on specific aspects
- Save important research outputs to `.sdlc/docs/category-feature-date.research.md` for future reference
