# Creating Demo Recordings

This guide explains how to create terminal demo recordings for LangDAG documentation.

## Using asciinema

### Installation

```bash
# macOS
brew install asciinema

# Linux
pip install asciinema
```

### Recording

```bash
# Start recording
asciinema rec demo.cast

# Run your demo commands
langdag chat new --model claude-sonnet-4-20250514
# ... interact with the CLI ...

# Press Ctrl+D to stop recording
```

### Upload to asciinema.org

```bash
asciinema upload demo.cast
```

### Convert to GIF (for README)

Install agg (asciinema gif generator):

```bash
# macOS
brew install agg

# Or from source
cargo install agg
```

Convert:

```bash
agg demo.cast demo.gif --theme monokai
```

### Recommended Settings

For best results:
- Terminal width: 100 columns
- Terminal height: 30 rows
- Font: JetBrains Mono or similar
- Theme: Dark (Monokai or Dracula)

## Using VHS (Alternative)

VHS from Charm lets you script terminal recordings:

### Installation

```bash
brew install vhs
```

### Create a tape file

```vhs
# demo.tape
Output demo.gif
Set FontSize 14
Set Width 1200
Set Height 600
Set Theme "Monokai"

Type "langdag chat new"
Enter
Sleep 2s

Type "What is LangDAG?"
Enter
Sleep 5s
```

### Generate

```bash
vhs demo.tape
```

## Demo Scenarios

### 1. Basic Conversation

```bash
langdag chat new
# Ask: "What is the capital of France?"
# Ask: "What's its population?"
langdag print <id>
```

### 2. Conversation Forking

```bash
langdag chat new
# Have a short conversation
langdag print <id>
# Fork from an earlier node
langdag chat continue <id> --node 2
```

### 3. Workflow Execution

```bash
langdag workflow run summarizer --input '{"text": "..."}' --stream
```

### 4. DAG Management

```bash
langdag ls
langdag show <id>
langdag print <id>
```
