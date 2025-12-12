# API Gateway for LLM Services

This project aims to build a client tool that acts as a unified gateway for various Large Language Model (LLM) providers. Starting with an MVP approach to validate core functionality.

## Technology Stack

The primary implementation will be in **Go**, with Python as an alternative option if deemed more suitable for specific requirements.

## Core Architecture

We'll first implement a dynamically configurable server that:
- Provides OpenAI-compatible endpoints
- Generates a unified URI base for clients to send requests
- Acts as both a CLI tool and a server service

## CLI Interface

Users can interact with the tool through command-line interface to:

### Configuration Management
- **Add**: Configure new providers with name, API base URL, and authentication token
  - All sensitive information must be securely stored in a fixed location
- **List**: View all configured providers
- **Delete**: Remove provider configurations

### Service Management
- Monitor and manage service status through CLI commands
- Implement appropriate service management solution

## Server Capabilities

The server provides:

1. **Unified API Endpoint**: A single URI base that any client can configure and use
2. **Token Management**: Generate and manage client authentication tokens
3. **Dynamic Configuration**: Load and refresh configurations in real-time
   - Important: Configuration changes made via CLI must be immediately reflected
4. **Request Proxying**: When clients use tokens and select models:
   - Server routes requests to the correct API base URL
   - Transforms and forwards authentication tokens
   - Handles the actual LLM provider request/response flow

## Key Features

- Real-time configuration updates without service restart
- Secure storage of API credentials
- OpenAI-compatible API interface for easy client integration
- Support for multiple LLM providers through a unified interface