# Documentation Skill

The `/doc` skill creates and manages project documentation including specs, API docs, README files, changelogs, and architecture documentation.

## Usage

```
/doc [type] [content]
```

**Types:**
- `spec` - Create specification documents
- `api` - Generate API documentation from code
- `readme` - Generate or update README files
- `changelog` - Create changelog entries
- `architecture` - Create architecture documentation
- `all` - Generate all documentation types

**Content:**
- File path, directory path, or content description depending on type

**Examples:**
- `/doc spec user-auth` - Create spec for user authentication feature
- `/doc api src/api` - Generate API docs for src/api
- `/doc readme` - Generate project README
- `/doc changelog "Added OAuth support"` - Add changelog entry
- `/doc architecture` - Create architecture documentation

## Guidelines

### When to Use
- **Spec Documents**: Before implementation to define requirements
- **API Documentation**: From specs or code to guide development
- **Architecture Docs**: When designing or updating system architecture
- **Changelog**: After completing features or bug fixes
- **README**: For project overview and setup instructions
- **API Changes**: Update API docs when endpoints change
- **New Features**: Document new functionality

### Spec Documentation

Create comprehensive specification documents:

```markdown
# [Feature Name] Specification

**Version**: 1.0
**Status**: Draft | In Review | Approved
**Last Updated**: YYYY-MM-DD

## Overview
[Brief description of the feature and its purpose]

## Objectives
- [Primary objective]
- [Secondary objective]

## Requirements

### Functional Requirements
1. **Requirement ID**: Description
   - Acceptance Criteria: [criteria]
   - Priority: High | Medium | Low

### Non-Functional Requirements
1. **Performance**: [specific requirements]
2. **Security**: [specific requirements]
3. **Scalability**: [specific requirements]

## API Design

### Endpoints
| Method | Endpoint | Description | Request | Response |
|--------|----------|-------------|---------|----------|
| GET | `/api/v1/resource` | List items | - | `Item[]` |
| POST | `/api/v1/resource` | Create item | `CreateItemRequest` | `Item` |

### Data Models
```typescript
interface Resource {
  id: string;
  name: string;
  // other fields
}
```

## Implementation Notes
[Technical considerations, dependencies, constraints]

## Testing Strategy
- Unit tests: [what to test]
- Integration tests: [what to test]
- E2E tests: [what to test]

## Rollout Plan
1. [Phase 1]
2. [Phase 2]
3. [Phase 3]
```

### Changelog Documentation

Create changelog entries following Keep a Changelog format:

```markdown
# Changelog

## [Unreleased]
### Added
- New feature description with issue reference

### Changed
- Modified functionality description

### Deprecated
- Feature being deprecated

### Removed
- Removed feature description

### Fixed
- Bug fix description with issue reference

### Security
- Security fix description
```

### Architecture Documentation

Create system architecture documentation:

```markdown
# System Architecture

## Overview
[High-level system description]

## Architecture Diagram
```
[ASCII or Mermaid diagram]
```

## Components
| Component | Responsibility | Technology |
|-----------|----------------|------------|
| [Name] | [Purpose] | [Tech] |

## Data Flow
[Describe how data flows through the system]

## Technology Stack
- Frontend: [tech]
- Backend: [tech]
- Database: [tech]
- Cache: [tech]
```

## Output Locations

| Type | Output Path | Format |
|------|-------------|--------|
| Spec | `./.sdlc/docs/spec/{feature-name}.md` | Markdown |
| API Docs | `./docs/api/{YYYY-MM-DD}-{resource}.md` | Markdown |
| README | `./README.md` | Markdown |
| Changelog | `./CHANGELOG.md` | Markdown |
| Architecture | `./.sdlc/docs/arch/{YYYY-MMDD}-arch.md` | Markdown |

## Documentation Principles

### Quality Standards
- **Accuracy**: Docs must match actual implementation
- **Completeness**: Document all public APIs and key functions
- **Clarity**: Use clear, concise language
- **Examples**: Provide usage examples for complex APIs
- **Maintainability**: Keep docs in sync with code changes

### Best Practices
1. **Document First**: Write docs before or during implementation
2. **Use Templates**: Follow consistent documentation patterns
3. **Include Examples**: Show, don't just tell
4. **Keep Current**: Update docs when code changes
5. **Review Regularly**: Check docs for accuracy

### Code Documentation Standards
- **JSDoc/TSDoc**: Use for TypeScript/JavaScript
- **Godoc**: Use for Go
- **Docstrings**: Use for Python
- **XML Docs**: Use for C#

## Process

### 1. Spec Documentation
1. Gather requirements from stakeholders or user stories
2. Define functional and non-functional requirements
3. Design API contracts and data models
4. Document implementation approach
5. Define testing strategy
6. Create spec document in `./.sdlc/docs/spec/`

### 2. API Documentation Generation
1. Scan target directory for API definitions
2. Extract routes, handlers, and type definitions
3. Generate structured API documentation
4. Save to appropriate docs location

### 3. README Generation
1. Analyze project structure and package.json
2. Extract key information (name, version, description)
3. Identify main features from code
4. Generate comprehensive README
5. Include installation and usage instructions

### 4. Changelog Creation
1. Review recent changes (git commits, PRs)
2. Categorize changes (Added, Changed, Fixed, etc.)
3. Format according to Keep a Changelog
4. Reference issue numbers
5. Update CHANGELOG.md

### 5. Architecture Documentation
1. Analyze system components and relationships
2. Map data flows and integration points
3. Document technology stack
4. Create architecture diagrams
5. Save to `./.sdlc/docs/arch/`

## Error Handling

- **Invalid doc type**: Provide list of valid types
- **Missing content**: Prompt for required information
- **File exists**: Ask to overwrite or append
- **Write failure**: Check directory permissions, create if needed
- **Invalid format**: Validate and suggest corrections

## Dependencies

- **cache**: Architecture knowledge for context
- **pencil**: Diagrams for complex API flows

## Completion Criteria

- [ ] All public APIs documented
- [ ] All key functions have inline docs
- [ ] README is comprehensive and current
- [ ] Examples provided for complex APIs
- [ ] Documentation is accurate and tested
