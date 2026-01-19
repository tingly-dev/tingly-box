import { ArrowLeftRight, Gauge, Globe, Layers } from "lucide-react";

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
    { icon: Globe, title: "Unified API", description: "Single configuration to connect hundreds of model providers." },
    { icon: Layers, title: "Load Balancing", description: "Distribute requests across multiple tokens by tactics." },
    {
        icon: ArrowLeftRight,
        title: "Auto API Translation",
        description: "Automatically translate API parameters among different providers."
    },
    { icon: Gauge, title: "High Performance", description: "Additional latency less than 1ms for seamless integration." },
];

export const faqs = [
    {
        question: "How does load balancing work?",
        answer: "Tingly Box automatically distributes requests across multiple API tokens using routing strategies. By default, it uses round-robin with a request threshold of 100 (if not configured)."
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
        answer: "LiteLLM validates the model name and expects it to be a known, supported model. When you set the model name to \"tingly\", LiteLLM treats it as invalid and throws this error. To work around this, configure the local model name as a real model such as \"gpt-3.5-turbo\". This is only to pass LiteLLM’s validation—the actual remote model used by tingly-box does not change."
    }
];