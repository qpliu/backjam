A Jamulus bot client that connects to a Jamulus server
(default: localhost:22124) and can stream mp3 files to
the server in response to commands in the chat.

To avoid the need for an Opus encoder, this will send raw 16-bit PCM
audio frames, which won't be supported by Jamulus until 4.0.0.

Build:
go build

Usage: ./backjam [CONFIG-FILE]

Documentation:

Configuration file: TBD

How to configure mp3 files: TBD

How to control the bot via Jamulus chat: TBD

The jamulus package is my first attempt at slop coding.  It's amazing
how much code can get generated and how wrong it can be.  It seems to
me it's a good way to quickly get started, but fixing all the missing
and incorrect code is best done manually.  I had hoped to be able to
avoid having to learn anything about the Jamulus protocol, but that
hope is dead.
