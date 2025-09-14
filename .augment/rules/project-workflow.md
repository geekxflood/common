---
type: "always_apply"
---

# Project Workflow Standards

This document defines the workflow standards, quality assurance processes, and project management practices for the Geekxflood Common project.

## Development Workflow

### Branch Strategy

Use a simplified Git flow with these branch types:

- **main**: Production-ready code, always deployable
- **feature/**: Feature development branches (`feature/add-hot-reload`)
- **fix/**: Bug fix branches (`fix/memory-leak-config`)
- **docs/**: Documentation-only changes (`docs/update-readme`)

### Branch Naming Conventions

```bash
# Feature branches
feature/package-name-feature-description
feature/config-granular-hot-reload
feature/logging-component-isolation

# Bug fix branches
fix/package-name-issue-description
fix/config-memory-leak
fix/ruler-race-condition

# Documentation branches
docs/description
docs/standardize-package-docs
docs/update-api-reference
```

### Commit Message Standards

Follow conventional commits format:

```
type(scope): description

[optional body]

[optional footer]
```

**Types:**
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `style`: Code style changes (formatting, etc.)
- `refactor`: Code refactoring
- `test`: Adding or updating tests
- `chore`: Maintenance tasks

**Examples:**
```
feat(config): add granular hot reload support

Add separate EnableSchemaHotReload and EnableConfigHotReload options
to allow independent control over file watching.

Closes #123

fix(logging): resolve race condition in component logger

The component logger had a race condition when accessing the internal
logger field concurrently. Added proper mutex protection.

docs(ruler): update API documentation

Update the ruler package documentation to reflect the new Inputs type
and simplified configuration structure.
```

## Development Process

### Feature Development

1. **Create Issue**: Document the feature requirement
2. **Create Branch**: Create feature branch from main
3. **Implement**: Write code following standards
4. **Test**: Add comprehensive tests
5. **Document**: Update relevant documentation
6. **Review**: Submit pull request for review
7. **Merge**: Merge after approval and CI passes

### Bug Fix Process

1. **Reproduce**: Create test case that reproduces the bug
2. **Fix**: Implement the minimal fix
3. **Test**: Ensure fix works and doesn't break anything
4. **Document**: Update docs if behavior changes
5. **Review**: Get code review
6. **Merge**: Merge after approval

## Quality Assurance

### Quality Gates

#### Pre-Commit Requirements

All code must pass these automated checks before commit:

```bash
# Code formatting
go fmt ./...

# Linting (must pass with 0 issues)
golangci-lint run --config .golangci.yml

# All tests must pass
go test ./...

# Race condition detection
go test -race ./...

# Security scanning
gosec -conf .gosec.json ./...

# Dead code detection
deadcode -test ./...
```

#### Pre-Merge Requirements

Before any code can be merged to main branch:

1. **All Quality Gates Pass**: Pre-commit checks must be green
2. **Code Review Approved**: At least one approving review required
3. **Documentation Updated**: Relevant docs must be updated
4. **Test Coverage**: Maintain or improve test coverage
5. **Performance Check**: No significant performance regressions

### Code Quality Standards

#### Complexity Limits

- **Cyclomatic Complexity**: Maximum 10 per function
- **Function Length**: Maximum 50 lines per function

#### Code Metrics

Monitor these metrics continuously:

- **Test Coverage**: Minimum 80%, target 90%
- **Code Duplication**: Maximum 5% duplicate code
- **Technical Debt**: Track and address technical debt regularly
- **Dependency Count**: Minimize external dependencies

#### Linting Configuration

Use `.golangci.yml` that is declared in this project. Never modify this file.

### Security Standards

#### Security Scanning

Regular security scans using:

- **gosec**: Static security analysis for Go
- **nancy**: Vulnerability scanner for dependencies
- **trivy**: Container and dependency vulnerability scanner

#### Security Best Practices

- **Input Validation**: Validate all external inputs
- **Error Handling**: Don't leak sensitive information in errors
- **Dependency Management**: Keep dependencies updated
- **Secrets Management**: Never commit secrets to version control
- **File Operations**: Use safe file operations with proper permissions

#### Vulnerability Response

1. **Detection**: Automated scanning in CI/CD pipeline
2. **Assessment**: Evaluate severity and impact
3. **Remediation**: Fix or mitigate vulnerabilities
4. **Testing**: Verify fixes don't break functionality
5. **Documentation**: Document security fixes in release notes

### Performance Standards

#### Performance Benchmarks

Maintain benchmarks for critical paths:

```go
func BenchmarkCriticalFunction(b *testing.B) {
    for i := 0; i < b.N; i++ {
        criticalFunction(testData)
    }
}
```

#### Performance Targets

- **Memory Allocations**: Minimize allocations in hot paths
- **CPU Usage**: Profile CPU-intensive operations
- **Response Time**: API calls should complete within acceptable time
- **Throughput**: Maintain or improve throughput metrics

#### Performance Monitoring

- **Continuous Benchmarking**: Run benchmarks in CI
- **Regression Detection**: Alert on performance regressions
- **Profiling**: Regular profiling of production workloads
- **Optimization**: Continuous performance optimization

## Code Review Process

### Pull Request Requirements

- [ ] **Clear Description**: Explain what and why
- [ ] **Linked Issues**: Reference related issues
- [ ] **Tests Added**: Include appropriate tests
- [ ] **Documentation Updated**: Update relevant docs
- [ ] **CI Passing**: All automated checks pass
- [ ] **Small Scope**: Keep PRs focused and small

### Review Guidelines

**For Authors:**
- Keep PRs small and focused (< 400 lines changed)
- Provide clear description and context
- Respond to feedback promptly and professionally
- Update PR based on feedback

**For Reviewers:**
- Review within 24 hours when possible
- Focus on correctness, clarity, and maintainability
- Provide constructive feedback
- Approve when satisfied with quality

### Review Checklist

- [ ] **Functionality**: Does the code do what it's supposed to do?
- [ ] **Tests**: Are there adequate tests for the changes?
- [ ] **Documentation**: Is documentation updated appropriately?
- [ ] **Standards**: Does code follow project standards?
- [ ] **Performance**: Are there any performance concerns?
- [ ] **Security**: Are there any security implications?

## Release Process

### Release Planning

- **Feature Releases**: Monthly minor releases
- **Patch Releases**: As needed for bug fixes
- **Major Releases**: Quarterly or when breaking changes needed

### Release Preparation

1. **Version Bump**: Update version numbers
2. **Changelog**: Update CHANGELOG.md
3. **Documentation**: Ensure docs are current
4. **Testing**: Run full test suite
5. **Security Scan**: Run security scans
6. **Performance Check**: Verify no regressions

### Release Execution

```bash
# Create release branch
git checkout -b release/v1.2.0

# Update version and changelog
# ... make changes ...

# Commit changes
git commit -m "chore: prepare release v1.2.0"

# Create pull request for release
# After approval and merge:

# Tag the release
git tag -a v1.2.0 -m "Release v1.2.0"
git push origin v1.2.0

# Publish release notes
# Create GitHub release with changelog
```

### Release Checklist

Before any release:

- [ ] All tests pass on all supported platforms
- [ ] Documentation is updated and accurate
- [ ] CHANGELOG is updated with all changes
- [ ] Version numbers are updated consistently
- [ ] Security scan passes with no high-severity issues
- [ ] Performance benchmarks show no regressions
- [ ] Breaking changes are clearly documented

### Version Management

Follow semantic versioning (SemVer):

- **MAJOR**: Breaking changes
- **MINOR**: New features, backward compatible
- **PATCH**: Bug fixes, backward compatible

### Release Notes

Include in every release:

- **New Features**: What's new and how to use it
- **Bug Fixes**: What was fixed
- **Breaking Changes**: What might break existing code
- **Deprecations**: What's being deprecated
- **Security Fixes**: Security-related fixes (without details)

### Post-Release

- **Monitor**: Watch for issues in the new release
- **Hotfixes**: Prepare hotfixes if critical issues found
- **Feedback**: Collect feedback from users
- **Planning**: Plan next release based on feedback

## Issue Management

### Issue Types

- **Bug**: Something is broken
- **Feature**: New functionality request
- **Enhancement**: Improvement to existing functionality
- **Documentation**: Documentation improvements
- **Question**: Support or clarification requests

### Issue Templates

**Bug Report:**

```markdown
## Bug Description
Brief description of the bug.

## Steps to Reproduce
1. Step one
2. Step two
3. Step three

## Expected Behavior
What should happen.

## Actual Behavior
What actually happens.

## Environment
- Go version:
- OS:
- Package version:

## Additional Context
Any other relevant information.
```

**Feature Request:**

```markdown
## Feature Description
Brief description of the requested feature.

## Use Case
Why is this feature needed? What problem does it solve?

## Proposed Solution
How should this feature work?

## Alternatives Considered
What other approaches were considered?

## Additional Context
Any other relevant information.
```

### Issue Lifecycle

1. **Triage**: Label and prioritize new issues
2. **Assignment**: Assign to appropriate team member
3. **Development**: Implement solution
4. **Review**: Code review and testing
5. **Verification**: Verify fix works
6. **Close**: Close issue when resolved

## CI/CD Pipeline

### Automation

Automate quality checks in CI/CD:

```yaml
# Example GitHub Actions workflow
name: Quality Assurance
on: [push, pull_request]
jobs:
  quality:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v3
      - name: Run quality checks
        run: |
          go fmt ./...
          golangci-lint run
          go test -race ./...
          gosec ./...
          deadcode ./...
```

### CI Pipeline Requirements

All tests must pass in CI before code can be merged:

1. **Unit Tests**: `go test ./...`
2. **Race Detection**: `go test -race ./...`
3. **Coverage Check**: Ensure coverage meets minimum threshold
4. **Benchmark Regression**: Check for performance regressions
5. **Integration Tests**: Run integration tests in appropriate environment

### Test Environment

- **Isolated**: Each test run should be isolated
- **Reproducible**: Same environment every time
- **Fast**: CI tests should complete quickly
- **Comprehensive**: Cover all supported platforms and Go versions

## Monitoring and Metrics

### Code Quality Metrics

Track these metrics over time:

- **Test Coverage Percentage**: Trend over time
- **Cyclomatic Complexity**: Average and maximum
- **Code Duplication**: Percentage of duplicate code
- **Technical Debt**: Time to fix technical debt
- **Bug Density**: Bugs per lines of code

### Process Metrics

Monitor development process:

- **Lead Time**: Time from commit to production
- **Deployment Frequency**: How often we deploy
- **Mean Time to Recovery**: Time to fix production issues
- **Change Failure Rate**: Percentage of deployments causing issues

### Development Metrics

Track these metrics to improve process:

- **Lead Time**: Time from issue creation to resolution
- **Cycle Time**: Time from development start to deployment
- **Code Review Time**: Time from PR creation to merge
- **Bug Escape Rate**: Bugs found in production vs. development

## Continuous Improvement

### Regular Reviews

Conduct regular reviews of:

- **Code Quality**: Monthly code quality reviews
- **Process Effectiveness**: Quarterly process reviews
- **Tool Effectiveness**: Annual tool reviews
- **Standards Updates**: Semi-annual standards updates

### Regular Retrospectives

Conduct retrospectives to improve process:

- **What Went Well**: Celebrate successes
- **What Could Improve**: Identify areas for improvement
- **Action Items**: Specific improvements to implement
- **Follow-up**: Ensure action items are completed

### Process Evolution

- **Experiment**: Try new tools and processes
- **Measure**: Measure impact of changes
- **Adapt**: Keep what works, discard what doesn't
- **Document**: Update process documentation

### Learning Culture

Foster continuous learning:

- **Knowledge Sharing**: Regular tech talks and demos
- **Code Reviews**: Learn from each other's code
- **External Learning**: Conferences, courses, books
- **Experimentation**: Encourage trying new approaches

### Feedback Loops

Establish feedback loops for:

- **Developer Experience**: Regular developer surveys
- **User Experience**: User feedback on APIs and tools
- **Performance**: Continuous performance monitoring
- **Security**: Regular security assessments

## Communication Standards

### Internal Communication

- **Daily Updates**: Brief status updates in team chat
- **Weekly Reviews**: Weekly progress reviews
- **Monthly Planning**: Monthly planning sessions
- **Quarterly Reviews**: Quarterly retrospectives

### External Communication

- **Release Notes**: Clear, user-focused release notes
- **Issue Updates**: Regular updates on issue progress
- **Documentation**: Keep public documentation current
- **Community**: Engage with community questions and feedback

## Tools and Automation

### Development Tools

- **IDE**: VS Code with Go extension
- **Linting**: golangci-lint with project configuration
- **Testing**: Go's built-in testing framework
- **Benchmarking**: Go's built-in benchmarking
- **Profiling**: pprof for performance analysis

### CI/CD Tools

- **Version Control**: Git with GitHub
- **CI/CD**: GitHub Actions
- **Quality Gates**: Automated quality checks
- **Security Scanning**: gosec and dependency scanning
- **Documentation**: Automated doc generation

### Project Management

- **Issue Tracking**: GitHub Issues
- **Project Planning**: GitHub Projects
- **Documentation**: Markdown files in repository
- **Communication**: Team chat and email

### Quality Dashboards

Maintain dashboards showing:

- **Build Status**: Current build health
- **Test Results**: Test pass/fail rates
- **Coverage Trends**: Test coverage over time
- **Security Status**: Security scan results
- **Performance Metrics**: Performance trends

## Incident Response

### Quality Incidents

When quality issues are discovered:

1. **Immediate Response**: Stop releases if critical
2. **Root Cause Analysis**: Understand why it happened
3. **Fix Implementation**: Implement proper fix
4. **Process Improvement**: Update processes to prevent recurrence
5. **Communication**: Inform stakeholders appropriately

### Post-Incident Review

After quality incidents:

- **Timeline**: Document what happened when
- **Root Cause**: Identify underlying causes
- **Action Items**: Specific improvements to implement
- **Follow-up**: Ensure action items are completed
