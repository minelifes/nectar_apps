import 'passkeys_stub.dart' if (dart.library.js_interop) 'passkeys_web.dart' as impl;

/// Passkeys (WebAuthn) in the browser: the web panel only. A passkey belongs to the panel's host name, and
/// the native apps cannot prove they belong to a self-hosted domain, so they sign in with a password.
bool get passkeysSupported => impl.supported();

/// navigator.credentials.create with the panel's options (JSON form); returns PublicKeyCredential.toJSON().
Future<Map<String, dynamic>> createPasskey(Map<String, dynamic> options) => impl.create(options);

/// navigator.credentials.get with the panel's options (JSON form); returns PublicKeyCredential.toJSON().
Future<Map<String, dynamic>> getPasskey(Map<String, dynamic> options) => impl.get(options);

/// The user closed the browser's passkey dialog (or no passkey exists for this site).
class PasskeyCancelled implements Exception {
  const PasskeyCancelled();

  @override
  String toString() => 'Cancelled';
}
