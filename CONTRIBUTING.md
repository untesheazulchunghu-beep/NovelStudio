# Contributing to NovelStudio

Thank you for your interest in contributing to NovelStudio!

## Code of Conduct

Be respectful, inclusive, and constructive. We welcome contributions from everyone.

## How to Contribute

### Reporting Bugs

1. Check if the bug has already been reported in Issues
2. Create a new issue with:
   - Clear title and description
   - Steps to reproduce
   - Expected vs actual behavior
   - System information (OS, VS version)
   - Screenshots if applicable

### Suggesting Features

1. Check existing feature requests
2. Create an issue with:
   - Clear use case
   - Expected behavior
   - Mockups or examples if applicable

### Pull Requests

1. **Fork the repository**
   ```bash
   git clone https://github.com/yourname/NovelStudio.git
   cd NovelStudio
   ```

2. **Create a feature branch**
   ```bash
   git checkout -b feature/your-feature-name
   ```

3. **Make your changes**
   - Follow the code style guide
   - Write or update tests
   - Update documentation

4. **Test your changes**
   ```bash
   cmake --build build --config Debug
   ctest --test-dir build
   ```

5. **Commit with conventional commits**
   ```bash
   git commit -m "feat: add chapter export to PDF"
   git commit -m "fix: resolve database connection leak"
   git commit -m "docs: update build instructions"
   ```

6. **Push and create PR**
   ```bash
   git push origin feature/your-feature-name
   ```

## Code Style

### C++ Guidelines

- Follow C++20 standards
- Use Modern C++ features (smart pointers, RAII, etc.)
- Prefer const correctness
- Use meaningful variable names
- Keep functions focused and small

### Formatting

```cpp
// Class example
class MyClass {
public:
    MyClass();
    ~MyClass();
    
    void PublicMethod();
    int GetValue() const { return value_; }
    
private:
    void PrivateMethod();
    
    int value_;
    std::string name_;
};

// Function example
bool ProcessData(const std::string& input) {
    if (input.empty()) {
        return false;
    }
    
    // Process...
    return true;
}
```

### Naming Conventions

- **Classes**: `PascalCase`
- **Functions**: `PascalCase`
- **Variables**: `camelCase`
- **Constants**: `UPPER_CASE`
- **Private members**: `camelCase_`
- **Files**: `snake_case.cpp` or `PascalCase.cpp`

## Testing

- Write unit tests for new features
- Ensure all tests pass before submitting PR
- Aim for meaningful test coverage

## Documentation

- Update README.md for user-facing changes
- Update docs/ for architectural changes
- Add code comments for complex logic
- Document public APIs with Doxygen comments

## Commit Message Format

```
type(scope): subject

body (optional)

footer (optional)
```

**Types:**
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation only
- `style`: Code style changes (formatting)
- `refactor`: Code restructuring
- `test`: Adding or updating tests
- `chore`: Build process or auxiliary tools

**Examples:**
```
feat(ai): add Claude AI provider support

Implements Anthropic Claude API integration with streaming support.

Closes #123
```

```
fix(database): prevent SQL injection in chapter query

Replace string concatenation with prepared statements.
```

## Development Setup

See [DEVELOPMENT.md](docs/DEVELOPMENT.md) for detailed setup instructions.

## Questions?

- Open a discussion on GitHub
- Check existing documentation
- Review closed issues for similar questions

Thank you for contributing! 🎉
