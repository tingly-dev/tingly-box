# Agents Tips and Lessons Learned

## Frontend Development

### Resource-Limited Environments
- **Do NOT run `npm run build` or `vite build`** on resource-limited machines (like Raspberry Pi, Docker containers with limited memory/CPU)
- Frontend builds with Vite/React require significant memory and CPU for bundling
- Alternative approaches:
  - Run `npx tsc --noEmit` for type checking only (much lighter)
  - Develop frontend on a more powerful machine, then deploy only the build artifacts
  - Use CI/CD pipelines for production builds

### MUI v7 Grid Component
- MUI v7 removed the original `Grid` component in favor of `Grid2`
- Use `Grid2` directly (import from `@mui/material/Grid2`) or use Box-based responsive layouts
- The Box-based approach with `sx={{ width: { xs: '100%', md: '50%' } }}` is more reliable and portable
- Avoid mixing Grid and Grid2 - pick one approach consistently

### TypeScript + React
- When using ternary operators inside Typography or other MUI components that expect a single child, wrap in Fragment `<>` to avoid TypeScript errors
- MUI v7's Typography may reject empty braces `{}` as children - use `<>content</>` instead

### Vite Configuration
- Always configure path aliases (`@/` → `src/`) in both:
  1. `vite.config.ts` with `resolve.alias`
  2. `tsconfig.json` with `compilerOptions.paths`
- Both are needed for proper IDE support and build resolution
