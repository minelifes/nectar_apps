import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:shared_preferences/shared_preferences.dart';

/// The interface language chosen on this device (null = follow the system).
final localeProvider = NotifierProvider<LocaleController, Locale?>(
  LocaleController.new,
);

class LocaleController extends Notifier<Locale?> {
  static const _key = 'nectar.locale';

  @override
  Locale? build() {
    Future.microtask(_load);
    return null;
  }

  Future<void> _load() async {
    final prefs = await SharedPreferences.getInstance();
    final code = prefs.getString(_key);
    state = code == null || code.isEmpty ? null : Locale(code);
  }

  Future<void> set(Locale? locale) async {
    state = locale;
    final prefs = await SharedPreferences.getInstance();
    await prefs.setString(_key, locale?.languageCode ?? '');
  }
}
