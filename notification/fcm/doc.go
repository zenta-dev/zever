// Package fcm provides a Firebase Cloud Messaging (FCM) push adapter.
//
// Configure via notification.Options.FCM:
//   - ProjectID (required): Firebase project ID.
//   - ServiceAccount (required): path to a service account JSON key file.
//     The path must be clean (filepath.Clean is a no-op), contain no ".."
//     element, end in .json, and not point at a directory.
package fcm
