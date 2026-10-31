import 'package:flutter/foundation.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:shared_preferences/shared_preferences.dart';

/// Keeps the access/refresh token pair.
///
/// Native apps use the platform keychain/keystore. If it is unavailable (e.g. a macOS build without the
/// keychain entitlement), tokens fall back to the app's preferences so sign-in still works.
/// Web builds only use tokens with the dev server (`NECTAR_API`); production web uses the cookie.
class TokenStore {
  static const _kAccess = 'nectar.access_token';
  static const _kRefresh = 'nectar.refresh_token';
  static const _secure = FlutterSecureStorage();

  String? access;
  String? refresh;

  bool get hasAny => access != null || refresh != null;

  Future<void> load() async {
    if (!kIsWeb) {
      try {
        access = await _secure.read(key: _kAccess);
        refresh = await _secure.read(key: _kRefresh);
        if (hasAny) return;
      } on Object {
        // keychain unavailable → preferences below
      }
    }
    final p = await SharedPreferences.getInstance();
    access = p.getString(_kAccess);
    refresh = p.getString(_kRefresh);
  }

  Future<void> save(String? accessToken, String? refreshToken) async {
    access = accessToken;
    refresh = refreshToken;
    if (!kIsWeb) {
      try {
        await _write(_kAccess, accessToken);
        await _write(_kRefresh, refreshToken);
        return;
      } on Object {
        // fall back to preferences
      }
    }
    final p = await SharedPreferences.getInstance();
    await _pref(p, _kAccess, accessToken);
    await _pref(p, _kRefresh, refreshToken);
  }

  Future<void> clear() async {
    access = null;
    refresh = null;
    if (!kIsWeb) {
      try {
        await _secure.delete(key: _kAccess);
        await _secure.delete(key: _kRefresh);
      } on Object {
        // ignore
      }
    }
    final p = await SharedPreferences.getInstance();
    await p.remove(_kAccess);
    await p.remove(_kRefresh);
  }

  static Future<void> _write(String key, String? value) => value == null
      ? _secure.delete(key: key)
      : _secure.write(key: key, value: value);

  static Future<void> _pref(
    SharedPreferences p,
    String key,
    String? value,
  ) async {
    if (value == null) {
      await p.remove(key);
    } else {
      await p.setString(key, value);
    }
  }
}
