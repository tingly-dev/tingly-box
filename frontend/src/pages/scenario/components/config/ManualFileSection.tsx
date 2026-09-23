import { Box, Tab, Tabs, Typography } from '@mui/material';
import React from 'react';
import CodeBlock from '@/components/CodeBlock';

export interface ManualFileTab {
    /** Tab strip label, e.g. "JSON" / "Windows" / "Linux/macOS" / "TOML". */
    label: string;
    value: string;
    /** File content shown for this tab (config text or a setup script). */
    code: string;
    language: string;
    filename: string;
    /** Label forwarded to copyToClipboard when this tab's code is copied. */
    copyLabel: string;
    maxHeight?: number;
    minHeight?: number;
    /** Prose rendered above the CodeBlock only while this tab is active. */
    description?: React.ReactNode;
}

interface ManualFileSectionProps {
    /** "Step N · ..." heading shown left of the tab strip. */
    heading: React.ReactNode;
    /** Prose rendered below the heading regardless of the active tab. */
    description?: React.ReactNode;
    tabs: ManualFileTab[];
    copyToClipboard: (text: string, label: string) => Promise<void>;
    /** Replaces the tab content while non-null (e.g. while config generates). */
    loading?: React.ReactNode;
}

// One "Step N · Create or update <file>" manual-setup section: heading +
// platform tab strip + a CodeBlock per tab. Shared by the ClaudeCode / Codex /
// Dsh / OpenCode config modals' manual tabs.
export const ManualFileSection: React.FC<ManualFileSectionProps> = ({
    heading,
    description,
    tabs,
    copyToClipboard,
    loading,
}) => {
    const [active, setActive] = React.useState(() => tabs[0]?.value ?? '');
    const activeTab = tabs.find((tab) => tab.value === active) ?? tabs[0];
    return (
        <Box sx={{ display: 'flex', flexDirection: 'column' }}>
            <Box sx={{ mb: 1, display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                <Typography variant="subtitle2" sx={{ color: 'text.secondary' }}>
                    {heading}
                </Typography>
                <Tabs
                    value={activeTab?.value ?? ''}
                    onChange={(_, value) => setActive(value)}
                    variant="standard"
                    sx={{ minHeight: 32, '& .MuiTabs-indicator': { height: 3 } }}
                >
                    {tabs.map((tab) => (
                        <Tab key={tab.value} label={tab.label} value={tab.value} sx={{ minHeight: 32, py: 0.5, fontSize: '0.875rem' }} />
                    ))}
                </Tabs>
            </Box>
            {description != null && (
                <Box sx={{ mb: 1.5 }}>
                    <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                        {description}
                    </Typography>
                </Box>
            )}
            <Box>
                {loading != null ? (
                    loading
                ) : activeTab && (
                    <>
                        {activeTab.description != null && (
                            <Box sx={{ mb: 2 }}>{activeTab.description}</Box>
                        )}
                        <CodeBlock
                            code={activeTab.code}
                            language={activeTab.language}
                            filename={activeTab.filename}
                            wrap={true}
                            onCopy={(code) => copyToClipboard(code, activeTab.copyLabel)}
                            maxHeight={activeTab.maxHeight}
                            minHeight={activeTab.minHeight}
                        />
                    </>
                )}
            </Box>
        </Box>
    );
};
