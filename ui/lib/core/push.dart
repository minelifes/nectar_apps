import 'dart:async';

import 'package:firebase_core/firebase_core.dart';
import 'package:firebase_messaging/firebase_messaging.dart';
import 'package:flutter/foundation.dart';

import '../api/client.dart';

/// Native push notifications (optional). The panel holds the Firebase project (Settings → Push
/// notifications); after sign-in the app asks GET /push/config and, when push is on, initializes Firebase
/// with those options (no google-services files in the build), asks for permission and registers its
/// device token. Android, iOS and macOS only; the web panel and desktop Linux/Windows skip it.
class PushService {
  PushService._();

  static final instance = PushService._();

  String? _token;
  bool _listening = false;
  StreamSubscription<String>? _refresh;

  /// Pages to open when a notification is tapped (the router listens).
  final _taps = StreamController<String>.broadcast();
  Stream<String> get taps => _taps.stream;

  static String? get _platform {
    if (kIsWeb) return null;
    return switch (defaultTargetPlatform) {
      TargetPlatform.android => 'android',
      TargetPlatform.iOS => 'ios',
      TargetPlatform.macOS => 'macos',
      _ => null,
    };
  }

  /// Registers this device with the panel when push is enabled there. Errors are swallowed: push is a
  /// convenience and must never break signing in.
  Future<void> register(Api api) async {
    final platform = _platform;
    if (platform == null) return;
    try {
      final cfg = await api.get('/push/config');
      if (cfg is! Map || cfg['enabled'] != true) return;
      final app =
          (platform == 'android' ? cfg['android'] : cfg['apple']) as Map?;
      final appId = '${app?['app_id'] ?? ''}';
      if (appId.isEmpty) return; // no Firebase app for this platform
      final bundle = '${app?['bundle_id'] ?? ''}'.trim();
      if (Firebase.apps.isEmpty) {
        await Firebase.initializeApp(
          options: FirebaseOptions(
            apiKey: '${app?['api_key'] ?? ''}',
            appId: appId,
            messagingSenderId: '${cfg['sender_id']}',
            projectId: '${cfg['project_id']}',
            iosBundleId: platform == 'android' || bundle.isEmpty
                ? null
                : bundle,
          ),
        );
      }
      final fm = FirebaseMessaging.instance;
      final perm = await fm.requestPermission();
      if (perm.authorizationStatus == AuthorizationStatus.denied) return;
      await fm.setForegroundNotificationPresentationOptions(
        alert: true,
        sound: true,
        badge: true,
      );
      final token = await fm.getToken();
      if (token == null) return;
      _token = token;
      await api.put(
        '/me/push-devices',
        data: {
          'token': token,
          'platform': platform,
          'name': _deviceName(platform),
        },
      );
      await _refresh?.cancel();
      _refresh = fm.onTokenRefresh.listen((t) async {
        _token = t;
        try {
          await api.put(
            '/me/push-devices',
            data: {
              'token': t,
              'platform': platform,
              'name': _deviceName(platform),
            },
          );
        } on Object {
          // retried at the next sign-in
        }
      });
      if (!_listening) {
        _listening = true;
        FirebaseMessaging.onMessageOpenedApp.listen(_open);
        final initial = await fm.getInitialMessage();
        if (initial != null) _open(initial);
      }
    } on Object catch (e) {
      debugPrint('push: $e');
    }
  }

  void _open(RemoteMessage m) {
    final path = m.data['path'];
    if (path is String && path.startsWith('/')) _taps.add(path);
  }

  /// Signing out: the device stops receiving this user's notifications.
  Future<void> unregister(Api? api) async {
    final token = _token;
    if (token == null || api == null) return;
    _token = null;
    await _refresh?.cancel();
    _refresh = null;
    try {
      await api.post('/me/push-devices/unregister', data: {'token': token});
    } on Object {
      // the session ends anyway; the panel drops tokens FCM reports as gone
    }
  }

  static String _deviceName(String platform) => switch (platform) {
    'android' => 'Android',
    'ios' => 'iPhone / iPad',
    _ => 'Mac',
  };
}
