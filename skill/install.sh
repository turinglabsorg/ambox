#!/bin/bash
set -e

TOOL_DIR="$HOME/.claude/tools/ambox"
SKILL_DIR="$HOME/.claude/skills"
CONFIG_DIR="$HOME/.ambox"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

echo "Installing Agent Mailbox (ambox) CLI..."

mkdir -p "$TOOL_DIR"
mkdir -p "$SKILL_DIR"
mkdir -p "$CONFIG_DIR" && chmod 700 "$CONFIG_DIR"

cp "$SCRIPT_DIR/index.js" "$TOOL_DIR/"
cp "$SCRIPT_DIR/package.json" "$TOOL_DIR/"

cd "$TOOL_DIR" && npm install --silent

mkdir -p "$SKILL_DIR/ambox"
cp "$SCRIPT_DIR/SKILL.md" "$SKILL_DIR/ambox/"

echo ""
echo "Installed successfully!"
echo "  Tool: $TOOL_DIR"
echo "  Skill: $SKILL_DIR/ambox"
echo ""

if [ ! -f "$CONFIG_DIR/config.json" ]; then
  echo "To register your agent, run:"
  echo "  node $TOOL_DIR/index.js register [--agent-id your-name]"
else
  echo "Config found at $CONFIG_DIR/config.json"
  echo "Already registered."
fi
