import { ArrowLeftRight, Gauge, Globe, Layers, Zap, Shield, LayoutDashboard, BarChart3 } from "lucide-react";

// External image URLs from GitHub repository
const screenshotBaseUrl = "https://raw.githubusercontent.com/tingly-dev/tingly-box/refs/heads/main/docs/images";

export const screenshots = [
    { src: `${screenshotBaseUrl}/1-home.png`, alt: "Tingly Box Home - Model Proxy Config" },
    { src: `${screenshotBaseUrl}/2-openai.png`, alt: "Tingly Box Credentials Management" },
    { src: `${screenshotBaseUrl}/4-select.png`, alt: "Tingly Box Proxy Rules Configuration" },
    // { src: `${screenshotBaseUrl}/xxx.png`, alt: "Tingly Box Server Status & Control" },
    // { src: `${screenshotBaseUrl}/xxx.png`, alt: "Tingly Box Activity Log & History" },
];

export const features = [
    { icon: Globe, title: "Unified API", description: "One mixin endpoint to rule them all — use OpenAI, Anthropic, or Google APIs interchangeably." },
    { icon: Zap, title: "Smart Routing", description: "Intelligently route requests across models and tokens based on cost, speed, or custom policies — not just simple load balancing." },
    { icon: Layers, title: "Smart Context Compression", description: "(Coming soon) Automatically distill context to its essential parts for sharper relevance, lower cost, and faster responses." },
    { icon: ArrowLeftRight, title: "Auto API Translation", description: "Seamlessly bridge OpenAI, Anthropic, Google, and other API dialects — no code changes needed." },
    { icon: Gauge, title: "Blazing Fast", description: "Adds typically < 1ms of overhead — so you get flexibility without latency tax." },
    { icon: Shield, title: "Flexible Auth", description: "Support for both API keys and OAuth (e.g., Claude.ai) — use your existing quotas anywhere." },
    { icon: LayoutDashboard, title: "Visual Control Panel", description: "Intuitive UI to manage providers, routes, aliases, and models at a glance." },
    { icon: BarChart3, title: "Client Side Usage Stats", description: "Track token consumption, latency, cost estimates, and model selection per request — directly from your client." },
];

export const faqs = [
    {
        question: "How does smart routing work?",
        answer: "Tingly Box intelligently routes requests across multiple API tokens and models using configurable strategies. Unlike simple load balancing, it considers cost, speed, and custom policies to optimize each request. By default, it uses round-robin with a request threshold of 100 (if not configured)."
    },
    {
        question: "Can I use OAuth providers like Claude Code?",
        answer: "Yes! You can add OAuth providers (like Claude Code) through the Web UI at http://localhost:12580. Once configured, requests route through your OAuth-authorized provider, using your existing quota instead of requiring a separate API key. This works with any OpenAI-compatible tool."
    },
    {
        question: "How do I enable mirrored networking mode in WSL2?",
        answer: "Add the following configuration to your .wslconfig file, then restart WSL:\n\n[wsl2]\nnetworkingMode=mirrored\n\nFor more details, see the official documentation: https://learn.microsoft.com/en-us/windows/wsl/wsl-config"
    },
    {
        question: "Can I use tingly-box running on the host for applications in Docker containers?",
        answer: "Yes. Just update the base URL to \n http://host.docker.internal:12580/{anthropic|openai} \nto connect from within the container."
    },
    {
        question: "Why do I get \"BadRequestError: LLM Provider NOT provided\" when using LiteLLM with model set to \"tingly\"?",
        answer: "LiteLLM validates the model name and expects it to be a known, supported model. When you set the model name to \"tingly\", LiteLLM treats it as invalid and throws this error. To work around this, configure the local model name as a real model such as \"gpt-3.5-turbo\". This is only to pass LiteLLM's validation—the actual remote model used by tingly-box does not change."
    }
];