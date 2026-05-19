A Jamulus bot client that takes an mp3 file, connects to a Jamulus
server (default: localhost:22124), sends the audio of the mp3 file
to the server, then disconnects.

Build: go build backjam.go jamulus.go stream.go

Usage: TBD

Additional envisioned features:

* time offset to start at
* adjust playback speed
* take a file with timings and strings to be sent to Jamulus chat
  at the given times, intended to be used to announce song sections
  and chords, for example:

```
   0:00.000: Count-in: Tempo: 130bpm Key: E Song: Baby-T
   0:01.846: Intro: N.C.×2 Next: E
   0:05.538: Chorus: E×4 Next: G
   0:12.923: Verse: G-D×3 C×2 Next: E
   0:22.154: Chorus: E×4 Next: G
   0:29.538: Verse: G-D×3 C×2 Next: F
   0:38.769: Bridge: (F B♭ G×2)×4 Next: E
   1:08.308: Chorus: E×8 Next: G
   1:23.077: Outro: G-D×7 C
```

* given a file with timings, be able to specify playback start at or
  before a named section
