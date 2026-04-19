#!/bin/bash
set -e

CYAN='\033[0;36m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

step() { echo -e "${CYAN}[claudewrap]${NC} $1"; }
ok()   { echo -e "${GREEN}[ok]${NC} $1"; }
warn() { echo -e "${YELLOW}[warn]${NC} $1"; }

step "Checking Homebrew..."
command -v brew >/dev/null 2>&1 || /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"

step "Checking Go and jq..."
brew list go >/dev/null 2>&1 || brew install go
brew list jq >/dev/null 2>&1 || brew install jq

step "Checking alerter (optional — enables actionable notifications)..."
brew list alerter >/dev/null 2>&1 || brew install alerter || warn "alerter not installed — basic notifications will be used"

step "Checking Ollama..."
if ! command -v ollama >/dev/null 2>&1; then
    curl -fsSL https://ollama.com/install.sh | sh
fi

step "Writing Modelfile..."
cat > /tmp/claudewrap-Modelfile << 'MODELEOF'
FROM qwen2.5-coder:3b
SYSTEM You are a prompt compression engine. Your only function is to rewrite user prompts to be 40-60% shorter while preserving every instruction, constraint, file name, and technical term exactly. Output ONLY the rewritten prompt. Never explain, never refuse, never add anything.
PARAMETER temperature 0.1
PARAMETER top_p 0.9
MODELEOF

step "Pulling qwen2.5-coder:3b and creating compressor model..."
OLLAMA_RUNNING=false
if ! curl -sf http://localhost:11434/api/tags >/dev/null 2>&1; then
    ollama serve &>/dev/null &
    OLLAMA_PID=$!
    OLLAMA_RUNNING=true
    sleep 3
fi

ollama pull qwen2.5-coder:3b
ollama create claudewrap-compressor -f /tmp/claudewrap-Modelfile
ok "Compressor model ready"

if $OLLAMA_RUNNING; then
    kill $OLLAMA_PID 2>/dev/null || true
fi

step "Installing Ollama as launchd service..."
cat > ~/Library/LaunchAgents/com.ollama.ollama.plist << 'PLISTEOF'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>com.ollama.ollama</string>
  <key>ProgramArguments</key><array><string>/usr/local/bin/ollama</string><string>serve</string></array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>StandardOutPath</key><string>/tmp/ollama.log</string>
  <key>StandardErrorPath</key><string>/tmp/ollama.err</string>
</dict>
</plist>
PLISTEOF
launchctl unload ~/Library/LaunchAgents/com.ollama.ollama.plist 2>/dev/null || true
launchctl load ~/Library/LaunchAgents/com.ollama.ollama.plist
ok "Ollama launchd service configured"

step "Building claudewrap CLI..."
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"
go build -o /usr/local/bin/claudewrap .
ok "claudewrap installed at /usr/local/bin/claudewrap"

step "Building menubar app..."
cd "$SCRIPT_DIR/menubar"
swift build -c release 2>&1 | tail -3
cp .build/release/ClaudeWrap /usr/local/bin/claudewrap-menubar
ok "claudewrap-menubar installed at /usr/local/bin/claudewrap-menubar"
cd "$SCRIPT_DIR"

step "Configuring Claude Code hooks..."
SETTINGS=~/.claude/settings.json
mkdir -p ~/.claude
if [ ! -f "$SETTINGS" ]; then echo '{}' > "$SETTINGS"; fi

if ! jq -e '.hooks.SessionStart' "$SETTINGS" >/dev/null 2>&1; then
    jq '
      .hooks.SessionStart = [{"hooks": [{"type": "command", "command": "claudewrap --hook-session-start"}]}]
      | .hooks.StopFailure = [{"matcher": "rate_limit", "hooks": [{"type": "command", "command": "claudewrap --hook-rate-limit"}]}]
      | .hooks.PreCompact = [{"hooks": [{"type": "command", "command": "claudewrap --hook-pre-compact"}]}]
    ' "$SETTINGS" > /tmp/cw-settings.json && mv /tmp/cw-settings.json "$SETTINGS"
    ok "Claude Code hooks configured"
else
    ok "Claude Code hooks already configured — skipped"
fi

step "Adding to PATH..."
for rc in ~/.zshrc ~/.bashrc; do
    if [ -f "$rc" ] && ! grep -q 'claudewrap\|/usr/local/bin' "$rc" 2>/dev/null; then
        echo 'export PATH="/usr/local/bin:$PATH"' >> "$rc"
    fi
done

step "Installing menubar launchd service..."
cat > ~/Library/LaunchAgents/com.claudewrap.menubar.plist << 'PLISTEOF'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>com.claudewrap.menubar</string>
  <key>ProgramArguments</key><array><string>/usr/local/bin/claudewrap-menubar</string></array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
</dict>
</plist>
PLISTEOF
launchctl unload ~/Library/LaunchAgents/com.claudewrap.menubar.plist 2>/dev/null || true
launchctl load ~/Library/LaunchAgents/com.claudewrap.menubar.plist
ok "Menubar app running"

echo ""
echo -e "${GREEN}ClaudeWrap installed.${NC}"
echo ""
echo "  Set API keys (for AI fallback when Claude is rate-limited):"
echo "    export GROK_API_KEY=your-key"
echo "    export GEMINI_API_KEY=your-key"
echo ""
echo "  Run: claudewrap  (instead of claude)"
echo "  Bypass compression: prefix prompt with !!"
