# Implementation plan

This projection is derived from Matt-native context and ADR documents.

## Problem

Download one complete television program from an Archive.org details URL using metadata-first discovery, resumable Go HTTP downloads, bounded concurrent fallback segments, and system FFmpeg concat assembly with retained diagnostics.

## Languages

go

## Decisions

- [Use Go-owned media downloads behind library adapters](docs/adr/0001-go-owned-media-downloads.md) — accepted
- [Assemble progressive segments with astiav and FFmpeg concat](docs/adr/0002-astiav-concat-remux.md) — superseded by ADR-0003
- [Assemble progressive segments with the system FFmpeg concat process](docs/adr/0003-system-ffmpeg-concat.md) — accepted
