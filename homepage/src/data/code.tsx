import {useState} from "react";
import {Button as MuiButton} from "@mui/material";
import {FaGithub} from "react-icons/fa";
import {ExternalLink} from "lucide-react";
// Syntax Highlighter Imports
import bash from 'react-syntax-highlighter/dist/esm/languages/prism/bash';
import json from 'react-syntax-highlighter/dist/esm/languages/prism/json';
import python from 'react-syntax-highlighter/dist/esm/languages/prism/python';
import SyntaxHighlighter from 'react-syntax-highlighter/dist/esm/prism-light';
import {oneDark} from 'react-syntax-highlighter/dist/esm/styles/prism';
import {Info, Sparkles, Settings2, MousePointerClick, RefreshCw} from 'lucide-react';

import screenshotProviderAdd from "@/assets/ProviderAdd.png";
import screenshotChooseProvider from "@/assets/ChooseProvider.png";

import {componentStyles} from "@/theme";

SyntaxHighlighter.registerLanguage('bash', bash);
SyntaxHighlighter.registerLanguage('python', python);
SyntaxHighlighter.registerLanguage('json', json);

const CodeBlock = ({code, language}: { code: string; language: string }) => {
    const [copied, setCopied] = useState(false);

    return (
        <div className="relative rounded-lg border w-full overflow-hidden bg-[#282c34]">
            <style dangerouslySetInnerHTML={{
                __html: `
                .force-wrap pre {
                    white-space: pre-wrap !important;
                    word-break: break-all !important;
                    overflow-wrap: anywhere !important;
                }
                @media (max-width: 768px) {
                    .force-wrap pre {
                        font-size: 0.65rem !important;
                        padding: 2rem 0.5rem 0.75rem 0.5rem !important;
                    }
                }
            `
            }}/>

            <div
                className="absolute top-0 left-2 px-2 py-0.5 text-[10px] font-bold rounded-b bg-gray-700 text-white z-10 uppercase">
                {language}
            </div>
            {/* Copy button */}
            <button
                onClick={() => {
                    navigator.clipboard.writeText(code);
                    setCopied(true);
                    setTimeout(() => setCopied(false), 2000);
                }}
                className="absolute top-2 right-2 p-1.5 rounded bg-gray-700 hover:bg-gray-600 text-white text-xs transition-colors z-10"
            >
                {copied ? 'Copied!' : 'Copy'}
            </button>

            <SyntaxHighlighter
                language={language}
                style={oneDark}
                wrapLines={true}
                wrapLongLines={true}
                className="force-wrap"
                customStyle={{
                    margin: 0,
                    padding: '2.5rem 0.75rem 1rem 0.75rem',
                    width: '100%',
                    boxSizing: 'border-box',
                    minHeight: 'auto',
                    fontSize: '0.75rem',
                    lineHeight: '1.5',
                    background: 'transparent',
                    overflowX: 'auto',
                    maxWidth: '100%',
                }}
            >
                {code}
            </SyntaxHighlighter>
        </div>
    );
};

export const STEPS = [
    {
        title: "Start the Service",
        desc: "Start the service by a quick npx start.",
        content: (
            <div className="space-y-4">
                <div className="space-y-2">
                    <p className="text-[10px] uppercase tracking-widest text-cyan-400 font-bold">
                        Start by Node.js
                    </p>
                    <CodeBlock language="bash" code="npx tingly-box@latest start"/>
                </div>

                {/*<div className="space-y-2">*/}
                {/*    <p className="text-[10px] uppercase tracking-widest text-blue-400 font-bold">*/}
                {/*        Option 2: Docker (Persistent)*/}
                {/*    </p>*/}
                {/*    <CodeBlock*/}
                {/*        language="bash"*/}
                {/*        code={`mkdir -p data/.tingly-box data/logs\ndocker run -d --name tingly-box \\\n  -p 12580:12580 \\\n  -v $(pwd)/data/.tingly-box:/app/.tingly-box \\\n  -v $(pwd)/data/logs:/app/logs \\\n  tingly-box:latest`}*/}
                {/*    />*/}
                {/*</div>*/}
            </div>
        ),
    },
    {
        title: "Add Providers",
        desc: "Configure API keys for your preferred model providers.",
        content: (
            <div className="space-y-4">
                <div className="space-y-2">
                    <div className={componentStyles.statusIndicator.container}>
                        <span className={componentStyles.statusIndicator.pulse}/>
                        Auto-Launching Homepage
                    </div>
                </div>

                {/* Added 'max-h' and 'object-top' to scale the taller empty-state image */}
                <div className={`${componentStyles.imageWithOverlay.container} overflow-hidden max-h-[180px] sm:max-h-[226px]`}>
                    <img
                        src={screenshotProviderAdd}
                        alt="Empty state for adding API keys"
                        className={`${componentStyles.imageWithOverlay.image} object-cover object-top scale-105`}
                    />
                    <div className={componentStyles.imageWithOverlay.overlay}>
                        <p className={componentStyles.imageWithOverlay.overlayText}>
                            <Sparkles size={12} className="text-cyan-400"/>
                            Look for the "Add Your First API Key" prompt on the Home screen.
                        </p>
                    </div>
                </div>

                <div className={componentStyles.infoBox.container}>
                    <Info size={18} className={`${componentStyles.infoBox.icon} mt-1`}/>
                    <div className="space-y-2">
                        <p className={componentStyles.infoBox.text}>
                            Once the page opens, click <b className="text-white">Add Your First API Key</b> to connect your preferred model providers.
                        </p>
                        {/* Integrated Reference Link */}
                        <div className="pt-2 border-t border-white/5 flex items-center gap-1.5">
                            <span className="text-xs text-muted-foreground">
                            Find more model configuration at{" "}
                                <a
                                    href="https://www.model-api.info/"
                                    target="_blank"
                                    rel="noopener noreferrer"
                                    className="text-cyan-400/80 hover:text-cyan-400 transition-colors inline-flex items-center gap-1 font-medium"
                                >
                                model-api.info <ExternalLink size={10} />
                            </a>
                            </span>
                        </div>
                    </div>
                </div>
            </div>
        ),
    },
    {
        title: "Setup Model Routing",
        desc: "Point your CLI tools and SDKs to local endpoints instead of default APIs.",
        content: (
            <div className="space-y-4">
                <div className="space-y-2">
                    <p className={`${componentStyles.sectionHeader} ${componentStyles.sectionHeaderMuted}`}>Python
                        SDK</p>
                    <CodeBlock
                        language="python"
                        code={`import openai\n\nclient = openai.OpenAI(\n    base_url = "http://localhost:12580/openai",\n    api_key = "YOUR_TINGLY_BOX_KEY"\n)\n\n# Use as before\nresponse = client.chat.completions.create(\n    model = "tingly",\n    messages = [{"role": "user", "content": "Hello!"}]\n)`}
                    />
                </div>
                <div className="space-y-2">
                    <p className={`${componentStyles.sectionHeader} ${componentStyles.sectionHeaderMuted}`}>
                        Claude Settings (~/.claude/settings.json)
                    </p>
                    <CodeBlock
                        language="json"
                        code={`{\n  "env": {\n    "ANTHROPIC_AUTH_TOKEN": "{tingly-box-token}",\n    "ANTHROPIC_BASE_URL": "http://localhost:12580/anthropic",\n    "ANTHROPIC_MODEL": "tingly"\n  }\n}`}
                    />
                </div>
            </div>
        ),
    },

    {
        title: "Choose Provider & Model",
        desc: "Access your UI homepage to select and activate your favorite models.",
        content: (
            <div className="space-y-4">
                {/* Step Guidance */}
                <div className="space-y-2">
                    <div className={componentStyles.statusIndicator.container}>
                        <span className={componentStyles.statusIndicator.pulse}/>
                        Configure Active Models
                    </div>
                </div>

                {/* Visual Guide with Screenshot */}
                <div className={componentStyles.imageWithOverlay.container}>
                    <img
                        src={screenshotChooseProvider}
                        alt="Provider and model selection interface"
                        className={componentStyles.imageWithOverlay.image}
                    />
                    <div className={componentStyles.imageWithOverlay.overlay}>
                        <p className={componentStyles.imageWithOverlay.overlayText}>
                            <MousePointerClick size={12} className="text-cyan-400"/>
                            Select a provider and click a model card to activate it.
                        </p>
                    </div>
                </div>

                {/* Detailed Instructions */}
                <div className={componentStyles.infoBox.container}>
                    <div className="flex flex-col gap-2">
                        <div className="flex items-start gap-2">
                            <div className="mt-1 bg-cyan-500/20 p-1 rounded">
                                <RefreshCw size={14} className="text-cyan-400"/>
                            </div>
                            <p className={componentStyles.infoBox.text}>
                                1. Click a provider (e.g., <b className="text-white">Anthropic</b>) and then click the <b
                                className="text-white">Fetch Model List</b> button to sync available models.
                            </p>
                        </div>
                        <div className="flex items-start gap-2">
                            <div className="mt-1 bg-cyan-500/20 p-1 rounded">
                                <Sparkles size={14} className="text-cyan-400"/>
                            </div>
                            <p className={componentStyles.infoBox.text}>
                                2. Simply click your favorite <b className="text-white">model name</b> (e.g., <b
                                className="text-white">claude-3-5-haiku-20241022</b>) to start routing requests.
                            </p>
                        </div>
                    </div>
                </div>
            </div>
        ),
    }
];