import 'package:flutter/foundation.dart';
import 'package:shared_preferences/shared_preferences.dart';

/// `flutter run -d chrome --dart-define=NECTAR_API=http://localhost:8080` points the dev server at a panel.
const _apiOverride = String.fromEnvironment('NECTAR_API');

/// Where the panel is and how to authenticate against it.
class ServerConfig {
  const ServerConfig({required this.baseUrl, required this.useTokens, required this.canChange});

  /// Panel origin without `/api` (null = native app that has not been connected yet).
  final String? baseUrl;

  /// Bearer tokens (native apps, web dev server) instead of the session cookie (web served by the panel).
  final bool useTokens;

  /// The user can switch servers (native apps only).
  final bool canChange;

  static const _key = 'nectar.server_url';

  static Future<ServerConfig> load() async {
    if (_apiOverride.isNotEmpty) {
      return ServerConfig(baseUrl: normalize(_apiOverride), useTokens: true, canChange: false);
    }
    if (kIsWeb) {
      // the panel serves this web app: same origin, cookie session
      return ServerConfig(baseUrl: Uri.base.origin, useTokens: false, canChange: false);
    }
    final prefs = await SharedPreferences.getInstance();
    return ServerConfig(baseUrl: prefs.getString(_key), useTokens: true, canChange: true);
  }

  static Future<void> save(String? url) async {
    final prefs = await SharedPreferences.getInstance();
    if (url == null) {
      await prefs.remove(_key);
    } else {
      await prefs.setString(_key, url);
    }
  }

  /// "panel.example.com/" → "https://panel.example.com"; also strips a trailing /api.
  static String normalize(String input) {
    var s = input.trim();
    if (!s.contains('://')) s = 'https://$s';
    while (s.endsWith('/')) {
      s = s.substring(0, s.length - 1);
    }
    if (s.endsWith('/api')) s = s.substring(0, s.length - 4);
    return s;
  }
}
