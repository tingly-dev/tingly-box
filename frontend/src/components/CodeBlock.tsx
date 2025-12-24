import { ContentCopy as CopyIcon, Check as CheckIcon } from '@mui/icons-material';
import { Box, IconButton, Typography } from '@mui/material';
import React, { useState } from 'react';
import { Highlight, themes } from 'prism-react-renderer';
import type { Language } from 'prism-react-renderer';

export interface CodeBlockProps {
    code: string;
    language?: string;
    filename?: string;
    showCopy?: boolean;
    onCopy?: (code: string) => void;
    sx?: React.CSSProperties;
    maxHeight?: number | string;
}

// Language mapping for Prism
const LANGUAGE_MAP: Record<string, Language> = {
    javascript: 'javascript',
    js: 'javascript',
    typescript: 'typescript',
    ts: 'typescript',
    jsx: 'jsx',
    tsx: 'tsx',
    json: 'json',
    yaml: 'yaml',
    yml: 'yaml',
    python: 'python',
    py: 'python',
    go: 'go',
    rust: 'rust',
    bash: 'bash',
    shell: 'bash',
    sh: 'bash',
    css: 'css',
    html: 'html',
    xml: 'markup',
    markdown: 'markdown',
    md: 'markdown',
    sql: 'sql',
};

const CodeBlock: React.FC<CodeBlockProps> = ({
    code,
    language = 'text',
    filename,
    showCopy = true,
    onCopy,
    sx = {},
    maxHeight = 400,
}) => {
    const [copied, setCopied] = useState(false);

    const handleCopy = async () => {
        if (onCopy) {
            onCopy(code);
        } else {
            try {
                await navigator.clipboard.writeText(code);
                setCopied(true);
                setTimeout(() => setCopied(false), 2000);
            } catch (err) {
                console.error('Failed to copy code:', err);
            }
        }
    };

    // Normalize language for Prism
    const prismLanguage = LANGUAGE_MAP[language.toLowerCase()] || 'markup';

    return (
        <Box
            sx={{
                position: 'relative',
                bgcolor: 'grey.900',
                borderRadius: 1,
                overflow: 'hidden',
                ...sx,
            }}
        >
            {/* Header bar with filename/language and copy button */}
            {(filename || language || showCopy) && (
                <Box
                    sx={{
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'space-between',
                        px: 1.5,
                        py: 0.75,
                        bgcolor: 'grey.800',
                        borderBottom: 1,
                        borderColor: 'grey.700',
                    }}
                >
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                        {filename && (
                            <Typography
                                variant="body2"
                                sx={{ fontFamily: 'monospace', color: 'grey.300', fontSize: '0.75rem' }}
                            >
                                {filename}
                            </Typography>
                        )}
                        {language && !filename && (
                            <Typography
                                variant="body2"
                                sx={{ fontFamily: 'monospace', color: 'grey.400', fontSize: '0.75rem' }}
                            >
                                {language}
                            </Typography>
                        )}
                    </Box>
                    {showCopy && (
                        <IconButton
                            size="small"
                            onClick={handleCopy}
                            sx={{ color: copied ? 'success.main' : 'grey.300', '&:hover': { bgcolor: 'grey.700' } }}
                            title={copied ? 'Copied!' : 'Copy code'}
                        >
                            {copied ? <CheckIcon fontSize="small" /> : <CopyIcon fontSize="small" />}
                        </IconButton>
                    )}
                </Box>
            )}
            {/* Code content with syntax highlighting */}
            <Box
                sx={{
                    overflowX: 'auto',
                    overflowY: 'auto',
                    maxHeight,
                }}
            >
                <Highlight
                    theme={themes.oneDark}
                    code={code}
                    language={prismLanguage}
                >
                    {({ className, style, tokens, getLineProps, getTokenProps }) => (
                        <pre
                            className={className}
                            style={{
                                ...style,
                                margin: 0,
                                padding: '1rem 1.5rem',
                                fontFamily: 'Monaco, Menlo, "Ubuntu Mono", "Consolas", source-code-pro, monospace',
                                fontSize: '0.75rem',
                                lineHeight: 1.5,
                                minWidth: '100%',
                            }}
                        >
                            {tokens.map((line, lineIndex) => (
                                <div {...getLineProps({ line, key: lineIndex })}>
                                    {line.map((token, tokenIndex) => (
                                        <span {...getTokenProps({ token, key: tokenIndex })} />
                                    ))}
                                </div>
                            ))}
                        </pre>
                    )}
                </Highlight>
            </Box>
        </Box>
    );
};

export default CodeBlock;
