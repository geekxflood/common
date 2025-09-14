---
type: "always_apply"
---

# Documentation Standards

This document defines the documentation standards for the Geekxflood Common project.

## Documentation Structure

### Package Documentation Template

Every package must follow this standardized structure:

```markdown
# Package `name`

Brief description of the package (1-2 sentences, no "production-grade" or marketing language).

## Key Features

✅ **Feature Name** - Brief description  
✅ **Another Feature** - Brief description  
✅ **Third Feature** - Brief description  

## Installation

```bash
go get github.com/geekxflood/common/packagename
```

## Architecture

Brief description of main components:

1. **Component One** - What it does
2. **Component Two** - What it does  
3. **Component Three** - What it does

## Package Structure

```text
packagename/
├── packagename.go      # Main implementation
├── packagename_test.go # Comprehensive unit tests
└── packagename.md      # This documentation file
```

## Quick Start

### Basic Usage

```go
// Simple, focused example showing core functionality
```

### Advanced Features (if applicable)

```go
// More complex examples for advanced use cases
```

## Configuration Reference

### Structure Definition

```go
// Show the main configuration structures
```

## API Reference

### Types

#### TypeName

```go
type TypeName struct {
    // Fields with comments
}
```

### Functions

#### FunctionName

```go
func FunctionName(params) (returns, error)
```

## Use Cases

### Real-World Example 1

```go
// Practical example showing actual usage
```

### Real-World Example 2

```go
// Another practical example
```

## Testing

Run tests:

```bash
cd packagename/
go test -v
go test -race
go test -bench=.
```

## Design Principles

1. **Principle One** - Brief explanation
2. **Principle Two** - Brief explanation
3. **Principle Three** - Brief explanation

## Dependencies

- `dependency/name` - Purpose and reason for inclusion
- Go standard library - `package1`, `package2`, `package3`

## Performance (if applicable)

Brief performance characteristics and optimization notes.
```

### README.md Structure

The main README must follow this structure:

```markdown
# Project Name

![Badges]

Brief project description.

## Packages

- [**package1**](#package-package1) - Brief description
- [**package2**](#package-package2) - Brief description

## Installation

```bash
go get commands
```

## Package `name`

Brief description matching package docs.

### Features

✅ **Feature** - Description

### Quick Start

```go
// Essential example
```

## Development

### Testing

### Code Quality

### Documentation

## License
```

## Writing Guidelines

### Language and Tone

- **Concise**: Be brief and to the point
- **Clear**: Use simple, direct language
- **Professional**: Avoid marketing language like "production-grade", "enterprise-level"
- **Consistent**: Use the same terminology throughout
- **Active Voice**: Prefer active over passive voice

### Formatting Standards

#### Headings

- Use consistent heading hierarchy
- Package names: `# Package \`name\``
- Main sections: `## Section Name`
- Subsections: `### Subsection Name`
- Sub-subsections: `#### Sub-subsection Name`

#### Lists

- Features: Use checkmarks `✅ **Feature Name** - Description`
- Principles: Use dashes `- **Principle** - Description`
- Dependencies: Use dashes with purpose `- \`package\` - Purpose`

#### Code Blocks

- Always specify language: `\`\`\`go`, `\`\`\`bash`, `\`\`\`yaml`
- Include complete, runnable examples
- Use proper indentation and formatting
- Add comments for clarity

#### Links

- Internal links: `[text](#section-name)`
- External links: `[text](https://example.com)`
- Package references: Use backticks `\`package\``

### Content Requirements

#### Code Examples

- **Complete**: Show full working examples
- **Practical**: Use real-world scenarios
- **Current**: Ensure examples match current API
- **Tested**: All examples should be tested and working

#### API Documentation

- **All Exported Items**: Document every exported function, type, and constant
- **Parameters**: Describe all parameters and their purpose
- **Return Values**: Explain what is returned
- **Errors**: Document possible error conditions
- **Examples**: Include usage examples

#### Error Messages

- **Descriptive**: Clearly explain what went wrong
- **Actionable**: Suggest how to fix the issue
- **Consistent**: Use consistent error message format
- **Context**: Include relevant context information

## Quality Assurance

### Documentation Review Checklist

- [ ] Follows standardized template structure
- [ ] Uses consistent formatting and style
- [ ] All code examples are complete and tested
- [ ] API documentation is comprehensive
- [ ] Links work correctly
- [ ] No spelling or grammar errors
- [ ] Matches current codebase functionality

### Automated Checks

- **Markdown Linting**: Use markdownlint for consistency
- **Link Checking**: Verify all links work
- **Code Example Testing**: Ensure examples compile and run
- **Spell Checking**: Use automated spell checking tools

### Maintenance

#### Regular Updates

- **API Changes**: Update docs immediately when API changes
- **New Features**: Document new features as they're added
- **Deprecations**: Mark deprecated features clearly
- **Examples**: Keep examples current with best practices

#### Version Control

- **Commit Messages**: Use clear commit messages for doc changes
- **Change Tracking**: Track significant documentation changes
- **Review Process**: All documentation changes require review

## Accessibility

### Writing for All Audiences

- **Beginner Friendly**: Include basic examples and explanations
- **Expert Accessible**: Provide advanced examples and edge cases
- **International**: Use clear, simple English
- **Screen Readers**: Use proper heading structure and alt text

### Visual Elements

- **Diagrams**: Use text-based diagrams when possible
- **Code Highlighting**: Ensure proper syntax highlighting
- **Contrast**: Maintain good contrast in any visual elements
- **Structure**: Use proper markdown structure for navigation

## Documentation Tools

### Recommended Tools

- **Editor**: Use editors with markdown preview and linting
- **Linting**: markdownlint for consistency checking
- **Spell Check**: Automated spell checking integration
- **Link Check**: Tools to verify link validity

### Integration

- **CI/CD**: Include documentation checks in CI pipeline
- **Pre-commit**: Run documentation checks before commits
- **Automated Updates**: Generate API docs from code comments
- **Publishing**: Automated publishing to documentation sites
