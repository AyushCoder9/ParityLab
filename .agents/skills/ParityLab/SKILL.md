```markdown
# ParityLab Development Patterns

> Auto-generated skill from repository analysis

## Overview
This skill teaches the core development patterns and conventions used in the ParityLab TypeScript codebase. You will learn about file naming, import/export styles, commit message conventions, and how to write and organize tests. This guide also provides suggested commands for common workflows to streamline your development process.

## Coding Conventions

### File Naming
- Use **camelCase** for file names.
  - Example: `userProfile.ts`, `dataFetcher.ts`

### Import Style
- Use **relative imports** for referencing modules within the project.
  - Example:
    ```typescript
    import { fetchData } from './dataFetcher';
    ```

### Export Style
- Use **named exports** for all modules.
  - Example:
    ```typescript
    // In dataFetcher.ts
    export function fetchData() { /* ... */ }

    // In another file
    import { fetchData } from './dataFetcher';
    ```

### Commit Messages
- Follow the **conventional commit** format.
- Use the `build` prefix for build-related changes.
  - Example:
    ```
    build: update dependencies to latest versions
    ```

## Workflows

### Code Contribution
**Trigger:** When adding, updating, or fixing code in the repository  
**Command:** `/contribute`

1. Create or update files using camelCase naming.
2. Use relative imports and named exports.
3. Write or update corresponding tests (see Testing Patterns).
4. Commit changes using the conventional commit format (e.g., `build: add new feature to dataFetcher`).
5. Push changes and open a pull request.

### Testing
**Trigger:** When verifying code correctness or before submitting changes  
**Command:** `/test`

1. Locate or create test files matching the `*.test.*` pattern (e.g., `userProfile.test.ts`).
2. Write tests for new or modified code.
3. Run the test suite using your preferred test runner (framework not detected; use appropriate TypeScript test tools).
4. Ensure all tests pass before committing.

## Testing Patterns

- Test files are named using the `*.test.*` pattern, such as `example.test.ts`.
- Place test files alongside the modules they test or in a dedicated test directory.
- Use the available TypeScript testing tools to write and run tests (testing framework not specified).
- Example test file structure:
  ```typescript
  // userProfile.test.ts
  import { getUserProfile } from './userProfile';

  describe('getUserProfile', () => {
    it('returns user data for valid ID', () => {
      // test implementation
    });
  });
  ```

## Commands
| Command      | Purpose                                      |
|--------------|----------------------------------------------|
| /contribute  | Start the code contribution workflow         |
| /test        | Run or write tests for the codebase          |
```
