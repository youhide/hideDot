# Contributing to hideDot

Thank you for your interest in contributing to hideDot! This document provides guidelines and instructions for contributing to the project.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Environment](#development-environment)
- [Making Contributions](#making-contributions)
- [Pull Request Process](#pull-request-process)
- [Coding Standards](#coding-standards)
- [Testing](#testing)

## Code of Conduct

By participating in this project, you are expected to uphold our Code of Conduct. Please report unacceptable behavior to the project maintainers.

## Getting Started

1. **Fork the repository**
   - Fork the hideDot repository on GitHub to your own account.

2. **Clone your fork**
   ```bash
   git clone https://github.com/YOUR-USERNAME/hidedot.git
   cd hidedot
   ```

3. **Add the upstream repository**
   ```bash
   git remote add upstream https://github.com/youhide/hidedot.git
   ```

## Development Environment

1. **Prerequisites**
   - Go (version 1.16 or later)
   - Git

2. **Building the project**
   ```bash
   go build
   ```

3. **Running tests**
   ```bash
   go test ./...
   ```

## Making Contributions

1. **Find an issue to work on**
   - Look for issues labeled `good first issue` or `help wanted`.
   - If you have a new idea, create an issue first to discuss it.

2. **Create a new branch**
   ```bash
   git checkout -b feature/your-feature-name
   ```
   - Use a descriptive branch name related to the issue or feature.

3. **Make your changes**
   - Follow the [Coding Standards](#coding-standards).
   - Add or update tests as necessary.
   - Add or update documentation for any user-facing changes.

4. **Commit your changes**
   ```bash
   git commit -m "Brief description of your changes"
   ```
   - Write clear, concise commit messages.
   - Reference issue numbers in your commit messages (e.g., "Fix #123: Add support for XYZ").

5. **Keep your branch updated**
   ```bash
   git fetch upstream
   git rebase upstream/main
   ```

## Pull Request Process

1. **Push your changes to your fork**
   ```bash
   git push origin feature/your-feature-name
   ```

2. **Open a pull request**
   - Go to the hideDot repository on GitHub.
   - Click "New Pull Request".
   - Select your fork and branch.
   - Provide a clear title and description for your PR.
   - Link related issues.

3. **Code review**
   - Wait for maintainers to review your PR.
   - Address any requested changes.
   - Keep the PR updated with the latest changes from main.

4. **PR approval and merge**
   - Once your PR is approved, a maintainer will merge it.
   - Your contribution will be part of the next release.

## Coding Standards

- Follow Go best practices and idiomatic Go.
- Use `gofmt` to format your code.
- Write clear comments for public functions and complex logic.
- Keep functions focused and small.
- Use meaningful variable and function names.

## Testing

- Add tests for new features.
- Ensure all tests pass before submitting a PR.
- Include both unit tests and integration tests where appropriate.
- Test edge cases and error conditions.

## Documentation

- Update documentation for any user-facing changes.
- Include examples for new features.
- Keep the README.md and other documentation up to date.

Thank you for contributing to hideDot! Your help is greatly appreciated.
