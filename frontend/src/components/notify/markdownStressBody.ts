// Exhaustive markdown document used to exercise platform renderers
// (Telegram MarkdownV2, Feishu rich text, …) end to end. Pre-filled as the
// default Body of the test-notification dialog so an operator can verify
// rendering with one click. Mirrors the Go-side
// remote/scenario/builtin/claudecode/markdown_test_body.go.
//
// It is NOT meant to read naturally — every block exists to exercise one
// rendering path. Grouped by category so a rendering bug can be localized
// to the element at fault.
export const MARKDOWN_STRESS_BODY = `# Heading H1
## Heading H2
### Heading H3

**bold** *italic* ~~strike~~ \`inline code\`

\`\`\`go
// fenced code block, language=go
func main() {
	fmt.Println("hello, markdown")
}
\`\`\`

\`\`\`
plain fenced block, no language
indentation should be preserved:
	ne	s	t	e	d
\`\`\`

- unordered item one
- unordered item two
  - nested under two
    - deeper nested
- back to top level

1. ordered first
2. ordered second
   1. nested ordered
   2. another nested
3. ordered third

- [ ] task unchecked
- [x] task checked

> This is a blockquote.
> It spans two lines.
>> nested blockquote line

| Feature | Supported | Notes |
|---------|:---------:|-------|
| Bold    | yes       | **on** |
| Code    | yes       | \`on\` |
| Table   | partial   | varies by platform |

---

[link text](https://example.com)
![alt text](https://example.com/image.png)

Inline specials that must not break the parser: a_b_c *not italic* [brackets] (parens) #hash atx ~tilde~

Emoji stress: 🎯 ✅ ❌ ⏳ 🔧 💭 ➜ — and a ZWJ couple 👨‍👩‍👧

A very long line with no break to test wrapping and truncation behaviour on platforms that cap message width — lorem ipsum dolor sit amet consectetur adipiscing elit sed do eiusmod tempor incididunt ut labore et dolore magna aliqua ut enim ad minim veniam quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat.


→ trailing blank lines above and below stress the paragraph splitter.`;
