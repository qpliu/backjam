A Jamulus bot client that connects to a Jamulus server
(default: localhost:22124) and can stream mp3 files to
the server in response to commands in the chat.

To avoid the need for an Opus encoder, this will send raw 16-bit PCM
audio frames, which won't be supported by Jamulus until 4.0.0.

Build:
go build

Usage: ./backjam [CONFIG-FILE]

Documentation is [here](SLOPUMENTATION.md), on

- how to configure the bot
- how to add audio files for the bot to play
- how to control the bot via Jamulus chat

The jamulus package is my first attempt at slop coding.  It's amazing
how much code can get generated and how wrong it can be.  It seems to
me it's a good way to quickly get started, but fixing all the missing
and incorrect code is best done manually.  I had hoped to be able to
avoid having to learn anything about the Jamulus protocol, but that
hope is dead.  However, I was very impressed by the documentation in
[SLOPUMENTATION.md](SLOPUMENTATION.md), which I just copied without
modification.  It does imply that flac or wav audio files are supported,
even though they are not, but they and other encodings easily could be,
should it become useful to support them.
