## TOML Configuration Files Documentation

### Overview

The **backjam** project reads TOML configuration files in two contexts:

1. **Main Configuration File** (`backjam.go`) - A single configuration file passed as a command-line argument
2. **Music File Configurations** (`file.go`) - Individual `.toml` files for each music track in the music directory

---

## 1. Main Configuration File (backjam.go)

### File Location
Passed as the first command-line argument to the program.

### File Structure

```toml
# Server connection configuration
Server = "localhost:22124"

# Client name for chat identification
ClientName = "backjam-bot"

# Base directory containing music files
Dir = "./music"

# List of authorized users who can control the bot
# If empty, all users are allowed
Users = [
  "user1",
  "user2"
]
```

### Configuration Properties

| Property | Type | Default | Description |
|----------|------|---------|-------------|
| `Server` | String | `"localhost:22124"` | The address and port of the streaming server to connect to. Format: `host:port` |
| `ClientName` | String | `"backjam-bot"` | The name used to identify the client when connecting to the server and sending chat messages. Used to filter out the bot's own messages. |
| `Dir` | String | `"./music"` | Base directory path where music configuration files (`.toml`) and audio files are located. Can be relative or absolute path. |
| `Users` | Array of Strings | Empty array | List of usernames allowed to send commands to the bot. If empty, all users are permitted. Users are checked against the sender in chat messages. |

### Usage Example

```bash
# Run with custom configuration
./backjam config.toml

# Default values are used if no config file is provided
./backjam
```

---

## 2. Music File Configurations (file.go)

### File Location
Stored in the directory specified by `Dir` configuration, with `.toml` extension.

**Example path:** `./music/song1.toml`

### File Structure

```toml
# Audio file path (relative to the music directory)
AudioFileName = "song1.mp3"

# Description displayed when the track is played
Description = "Now playing: Song 1 by Artist"

# Tempo in beats per minute (BPM)
Tempo = 120

# Time signature configuration
[Bar]
Beats = 4

# Initial offset in milliseconds before playback starts
StartOffsetMs = 0

# Chat messages synchronized with specific beats or bars
[[ChatMessages]]
Tag = "intro"
Text = "🎵 Intro starting!"
Beat = 1

[[ChatMessages]]
Tag = "verse1"
Text = "🎤 First verse"
Bar = 5

[[ChatMessages]]
Tag = "chorus"
Text = "🎶 Chorus!"
Beat = 32
```

### Music File Properties

| Property | Type | Default | Description |
|----------|------|---------|-------------|
| `AudioFileName` | String | Required | Path to the audio file, relative to the music directory (e.g., `"song.mp3"`, `"subfolder/song.wav"`). |
| `Description` | String | Required | Text displayed in chat when the track starts playing. Can include track metadata or emojis. |
| `Tempo` | Integer | Required | Beats per minute (BPM) of the track. Must be > 0 for chat messages to be calculated. If ≤ 0, no chat messages are sent. |
| `Bar.Beats` | Integer | `4` | Number of beats per bar/measure. Defaults to 4 if not specified or ≤ 0. Used for timing calculations. |
| `StartOffsetMs` | Integer | `0` | Delay in milliseconds before timing begins. Used to sync chat messages if there's an initial silence or delay in the audio file. |
| `ChatMessages` | Array of Objects | Empty array | Array of chat messages synchronized to the track. See details below. |

### ChatMessages Sub-Properties

Each message in the `ChatMessages` array contains:

| Property | Type | Description |
|----------|------|-------------|
| `Tag` | String | Unique identifier for the message (e.g., `"intro"`, `"verse1"`). Used internally for tracking. |
| `Text` | String | The actual message text to display in chat. Can include emojis and formatting. |
| `Beat` | Integer | Beat number at which to send the message (1-indexed). Takes precedence if both `Beat` and `Bar` are specified. |
| `Bar` | Integer | Bar/measure number at which to send the message (1-indexed). Used if `Beat` is not specified. |

### Timing Calculation

- **Beat Duration:** `1 minute / Tempo`
- **Bar Duration:** `Bar.Beats * Beat Duration` (default 4 beats)
- **Message Timestamp:** 
  - If `Bar` specified: `(Bar - 1) * BarDuration + StartOffsetMs`
  - Else if `Beat` specified: `(Beat - 1) * BeatDuration + StartOffsetMs`

### Example Configurations

#### Simple Track
```toml
AudioFileName = "track.mp3"
Description = "My first track"
Tempo = 100
StartOffsetMs = 500

[[ChatMessages]]
Tag = "start"
Text = "🎵 Starting track"
Beat = 1
```

#### Complex Track with Multiple Messages
```toml
AudioFileName = "album/song.flac"
Description = "Album - Song Title (3:45)"
Tempo = 128

[Bar]
Beats = 3

StartOffsetMs = 250

[[ChatMessages]]
Tag = "intro"
Text = "Intro"
Bar = 1

[[ChatMessages]]
Tag = "verse"
Text = "Verse 1"
Beat = 13

[[ChatMessages]]
Tag = "chorus"
Text = "Chorus"
Bar = 9

[[ChatMessages]]
Tag = "bridge"
Text = "Bridge"
Bar = 17

[[ChatMessages]]
Tag = "outro"
Text = "Outro"
Beat = 129
```

### File Discovery

- The system scans the directory recursively for `.toml` files
- File names (without `.toml` extension) are used as identifiers for the `.ls` and `.p` commands
- Subdirectories are preserved in the path structure

---

## Command Reference

Users interact with music files through chat commands:

| Command | Format | Description |
|---------|--------|-------------|
| `.ls` | `.ls [prefix]` | List available tracks and directories, optionally filtered by prefix |
| `.p` | `.p [filename]` | Play a track. `.p` alone stops playback. |
| `.x` | `.x` | Exit the bot |
