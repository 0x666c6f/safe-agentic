Update all dependencies to their latest stable versions:
1. Check for outdated packages (npm outdated, pip list --outdated, go list -m -u all, etc.)
2. Update each dependency one at a time
3. Run tests after each update to catch breaking changes
4. Fix any breaking changes introduced by the update
5. Commit each update separately with a descriptive message
6. Push all changes and create a PR titled "chore: update dependencies"
