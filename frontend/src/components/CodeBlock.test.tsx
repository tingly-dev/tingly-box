import { render } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import CodeBlock from './CodeBlock';

const tokenClasses = (container: HTMLElement) =>
    new Set(Array.from(container.querySelectorAll('.token')).flatMap((el) => Array.from(el.classList)));

describe('CodeBlock highlighting', () => {
    it('highlights shell code, which prism-react-renderer does not bundle', () => {
        const { container } = render(<CodeBlock code={'if [ -f go.mod ]; then\n  go test ./...\nfi'} language="bash" />);
        expect(tokenClasses(container).has('keyword')).toBe(true);
    });

    it('highlights diffs', () => {
        const { container } = render(<CodeBlock code={'- old line\n+ new line'} language="diff" />);
        const classes = tokenClasses(container);
        expect(classes.has('deleted')).toBe(true);
        expect(classes.has('inserted')).toBe(true);
    });

    it('renders an unknown language as plain text instead of coloring it as HTML', () => {
        const { container } = render(<CodeBlock code={'<div> in a log line'} language="log" />);
        expect(tokenClasses(container).has('tag')).toBe(false);
    });
});
