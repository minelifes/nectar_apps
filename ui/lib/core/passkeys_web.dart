import 'dart:convert';
import 'dart:js_interop';
import 'dart:js_interop_unsafe';

import 'passkeys.dart';

@JS('JSON.parse')
external JSAny? _parse(JSString s);

@JS('JSON.stringify')
external JSString _stringify(JSAny? o);

@JS('PublicKeyCredential')
external JSObject? get _publicKeyCredential;

@JS('navigator.credentials')
external JSObject? get _credentials;

/// Needs the JSON helpers of WebAuthn Level 3 (Chrome/Edge 129+, Safari 18+, Firefox 119+).
bool supported() {
  final pkc = _publicKeyCredential;
  return pkc != null &&
      _credentials != null &&
      pkc.has('parseCreationOptionsFromJSON') &&
      pkc.has('parseRequestOptionsFromJSON');
}

Future<Map<String, dynamic>> _ceremony(
  String parse,
  String method,
  Map<String, dynamic> options,
) async {
  final publicKey = _publicKeyCredential!.callMethod<JSObject>(
    parse.toJS,
    _parse(jsonEncode(options).toJS),
  );
  final arg = JSObject()..setProperty('publicKey'.toJS, publicKey);
  JSObject? cred;
  try {
    cred = await _credentials!
        .callMethod<JSPromise<JSObject?>>(method.toJS, arg)
        .toDart;
  } on Object catch (e) {
    // NotAllowedError: the dialog was closed, timed out, or there is no passkey for this site
    if (e.toString().contains('NotAllowedError') ||
        e.toString().contains('AbortError')) {
      throw const PasskeyCancelled();
    }
    rethrow;
  }
  if (cred == null) throw const PasskeyCancelled();
  return jsonDecode(_stringify(cred.callMethod<JSAny?>('toJSON'.toJS)).toDart)
      as Map<String, dynamic>;
}

Future<Map<String, dynamic>> create(Map<String, dynamic> options) =>
    _ceremony('parseCreationOptionsFromJSON', 'create', options);

Future<Map<String, dynamic>> get(Map<String, dynamic> options) =>
    _ceremony('parseRequestOptionsFromJSON', 'get', options);
