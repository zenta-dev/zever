// Package cdntest provides a conformance kit for cdn.CDN implementations.
//
// Call Conformance with a factory that returns a fresh CDN per subtest. The
// kit covers purge by URL, purge by tag, purge-all, and Name. Adapters wire
// it into their own tests, mirroring core/cache/cachetest and
// core/session/sessiontest.
package cdntest
