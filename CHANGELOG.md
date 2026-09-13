# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Initial project scaffolding: Go module, CI, lint configuration, Makefile, and
  open source project files.
- Generic `codec` package: `Encoder[V]` / `Decoder[V]` / `Codec[V]` interfaces
  with a `JSONCodec[V]` implementation backed by `encoding/json/v2` and
  sentinel `ErrEncode` / `ErrDecode` errors.
- Generic `log` package: `Logger` / `Event` / `Context` facade with a
  registration-based adapter system and `noop`, stdlib `slog`, and `zerolog`
  backends.
- Generic `cache` package: `Cache` facade with `Typed[K, V]` codec helpers and
  in-memory LRU plus Redis-backed adapters.
- Generic `queue` package: `Queue` facade with in-memory and Redis-backed
  adapters for topic-based messaging.
