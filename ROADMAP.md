# RemoteTerm Roadmap

This roadmap outlines major upcoming features and improvements for RemoteTerm. As with any roadmap, priorities and timelines may shift as development progresses.

Want input on the roadmap? Join the discussion on [Discord](https://discord.gg/XfvZ334gwU).

Legend: ✅ Done | 🔧 In Progress | 🔷 Planned | 🤞 Stretch Goal

## Current AI Capabilities

RemoteTerm's AI assistant is already powerful and continues to evolve. Here's what works today:

### AI Provider Support

- ✅ OpenAI (including gpt-5 and gpt-5-mini models)
- ✅ Google Gemini (v0.13)
- ✅ OpenRouter and custom OpenAI-compatible endpoints (v0.13)
- ✅ Azure OpenAI (modern and legacy APIs) (v0.13)
- ✅ Local AI models via Ollama, LM Studio, vLLM, and other OpenAI-compatible servers (v0.13)

### Context & Input

- ✅ Widget context integration - AI sees your open terminals, web views, and other widgets
- ✅ Image and document upload - Attach images and files to conversations
- ✅ Local file reading - Read text files and directory listings on local machine
- ✅ Web search - Native web search capability for current information
- ✅ Shell integration awareness - AI understands terminal state (shell, version, OS, etc.)

### Widget Interaction Tools

- ✅ Widget screenshots - Capture visual state of any widget
- ✅ Terminal scrollback access - Read terminal history and output
- ✅ Web navigation - Control browser widgets

## ROADMAP Enhanced AI Capabilities

### AI Configuration & Flexibility

- ✅ BYOK (Bring Your Own Key) - Use your own API keys for any supported provider (v0.13)
- ✅ Local AI agents - Run AI models locally on your machine (v0.13)
- 🔧 Enhanced provider configuration options
- 🔷 Context (add markdown files to give persistent system context)

### Expanded Provider Support

- 🔷 Anthropic Claude - Full integration with extended thinking and tool use

### Advanced AI Tools

#### File Operations

- ✅ AI file writing with intelligent diff previews
- ✅ Rollback support for AI-made changes
- 🔷 Multi-file editing workflows
- 🔷 Safe file modification patterns

#### Terminal Command Execution

- 🔧 Execute commands directly from AI
- ✅ Intelligent terminal state detection
- 🔧 Command result capture and parsing

### Remote & Advanced Capabilities

- 🔷 Remote file operations - Read and write files on SSH connections
- 🔷 Custom AI-powered widgets (Tsunami framework)
- 🔷 AI Can spawn RemoteTerm Blocks
- 🔷 Drag&Drop from Preview Widgets to RemoteTerm AI

### RemoteTerm AI Widget Builder

- 🔷 Visual builder for creating custom AI-powered widgets
- 🔷 Template library for common AI workflows
- 🔷 Rapid prototyping and iteration tools

## Other Platform & UX Improvements (Non AI)

- 🔷 Import/Export tab layouts and widgets
- 🔧 Enhanced layout actions (splitting, replacing blocks)
- 🔷 Extended drag & drop for files/URLs
- 🔷 Tab templates for quick workspace setup
- 🔷 Advanced keybinding customization
  - 🔷 Widget launch shortcuts
  - 🔷 System keybinding reassignment
- 🔷 Command Palette
- 🔷 Monaco Editor theming
