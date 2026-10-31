import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:shared_preferences/shared_preferences.dart';

/// Light / dark / system, remembered on this device.
final themeModeProvider = NotifierProvider<ThemeModeController, ThemeMode>(
  ThemeModeController.new,
);

class ThemeModeController extends Notifier<ThemeMode> {
  static const _key = 'nectar.theme_mode';

  @override
  ThemeMode build() {
    Future.microtask(_load);
    return ThemeMode
        .dark; // dark-first brand; the saved choice loads right after
  }

  Future<void> _load() async {
    final prefs = await SharedPreferences.getInstance();
    final saved = prefs.getString(_key);
    state = ThemeMode.values.firstWhere(
      (m) => m.name == saved,
      orElse: () => ThemeMode.dark,
    );
  }

  Future<void> set(ThemeMode mode) async {
    state = mode;
    final prefs = await SharedPreferences.getInstance();
    await prefs.setString(_key, mode.name);
  }
}
