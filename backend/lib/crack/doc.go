// Package crack provides protocol brute-force and unauthorized-access checks
// that can be called independently from the portscan package.
//
// Typical entrypoints:
//   - SupportedProtocols()
//   - Run(target, credentials, callback)
//   - RunWithContext(ctx, target, credentials, callback)
//   - RegisterScanner(protocol, scanFunc)
package crack
