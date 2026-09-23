# go-archorg-dl

This context records the project vocabulary. Keep definitions precise and free of implementation details.

## Language

**Program**:
A single television broadcast or episode represented by an Archive.org item.
_Avoid_: show, recording

**Archive Item**:
The Archive.org resource identified by the user's details URL and containing metadata and media files.
_Avoid_: page, asset

**Candidate Video**:
A video file selected from an Archive Item as the source for the Program.
_Avoid_: best file, media

**Segment**:
A time-bounded portion of a Program that can be independently retrieved and later assembled in order.
_Avoid_: chunk, fragment

**Published Output**:
The complete MP4 file made available to the user after a successful Program download.
_Avoid_: final file, result
