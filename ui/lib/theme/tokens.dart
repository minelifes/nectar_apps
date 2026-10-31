import 'package:flutter/material.dart';

/// Brand tokens (PLAN.md §9). Dark-first; the light palette mirrors it.
abstract final class NectarTokens {
  // accent
  static const amber = Color(0xFFF5A524);
  static const amberHover = Color(0xFFFFB940);
  static const pollen = Color(0xFFFFD166);
  static const onAmber = Color(0xFF241500);

  // status
  static const running = Color(0xFF3DD68C);
  static const deploying = Color(0xFF5B8DEF);
  static const warning = Color(0xFFF5A524);
  static const failed = Color(0xFFF2555A);
  static const stopped = Color(0xFF8A8F98);

  // dark surfaces
  static const darkBackground = Color(0xFF0E0F12);
  static const darkSurface = Color(0xFF16181D);
  static const darkElevated = Color(0xFF1E2128);
  static const darkBorder = Color(0xFF2A2E37);
  static const darkText = Color(0xFFE8E9EC);
  static const darkMuted = Color(0xFF8A8F98);

  // light surfaces
  static const lightBackground = Color(0xFFF6F6F3);
  static const lightSurface = Color(0xFFFFFFFF);
  static const lightElevated = Color(0xFFFBFAF7);
  static const lightBorder = Color(0xFFE4E2DC);
  static const lightText = Color(0xFF16181D);
  static const lightMuted = Color(0xFF666B75);

  // shape & spacing
  static const radius = 12.0;
  static const radiusSmall = 8.0;
  static const gap = 16.0;
  static const gapLarge = 24.0;

  // type
  static const fontUi = 'Inter';
  static const fontDisplay = 'SpaceGrotesk';
  static const fontMono = 'JetBrainsMono';

  // motion
  static const fast = Duration(milliseconds: 150);
  static const medium = Duration(milliseconds: 280);
}

/// Colors that are not part of Material's ColorScheme. Read with `NectarColors.of(context)`.
@immutable
class NectarColors extends ThemeExtension<NectarColors> {
  const NectarColors({
    required this.background,
    required this.surface,
    required this.elevated,
    required this.border,
    required this.text,
    required this.muted,
    required this.running,
    required this.deploying,
    required this.warning,
    required this.failed,
    required this.stopped,
  });

  final Color background;
  final Color surface;
  final Color elevated;
  final Color border;
  final Color text;
  final Color muted;
  final Color running;
  final Color deploying;
  final Color warning;
  final Color failed;
  final Color stopped;

  static const dark = NectarColors(
    background: NectarTokens.darkBackground,
    surface: NectarTokens.darkSurface,
    elevated: NectarTokens.darkElevated,
    border: NectarTokens.darkBorder,
    text: NectarTokens.darkText,
    muted: NectarTokens.darkMuted,
    running: NectarTokens.running,
    deploying: NectarTokens.deploying,
    warning: NectarTokens.warning,
    failed: NectarTokens.failed,
    stopped: NectarTokens.stopped,
  );

  static const light = NectarColors(
    background: NectarTokens.lightBackground,
    surface: NectarTokens.lightSurface,
    elevated: NectarTokens.lightElevated,
    border: NectarTokens.lightBorder,
    text: NectarTokens.lightText,
    muted: NectarTokens.lightMuted,
    running: Color(0xFF1FA968),
    deploying: Color(0xFF3B6FD9),
    warning: Color(0xFFD9890A),
    failed: Color(0xFFD93A40),
    stopped: Color(0xFF7A7F88),
  );

  static NectarColors of(BuildContext context) =>
      Theme.of(context).extension<NectarColors>() ??
      (Theme.of(context).brightness == Brightness.dark ? dark : light);

  @override
  NectarColors copyWith({
    Color? background,
    Color? surface,
    Color? elevated,
    Color? border,
    Color? text,
    Color? muted,
    Color? running,
    Color? deploying,
    Color? warning,
    Color? failed,
    Color? stopped,
  }) => NectarColors(
    background: background ?? this.background,
    surface: surface ?? this.surface,
    elevated: elevated ?? this.elevated,
    border: border ?? this.border,
    text: text ?? this.text,
    muted: muted ?? this.muted,
    running: running ?? this.running,
    deploying: deploying ?? this.deploying,
    warning: warning ?? this.warning,
    failed: failed ?? this.failed,
    stopped: stopped ?? this.stopped,
  );

  @override
  NectarColors lerp(ThemeExtension<NectarColors>? other, double t) {
    if (other is! NectarColors) return this;
    return NectarColors(
      background: Color.lerp(background, other.background, t)!,
      surface: Color.lerp(surface, other.surface, t)!,
      elevated: Color.lerp(elevated, other.elevated, t)!,
      border: Color.lerp(border, other.border, t)!,
      text: Color.lerp(text, other.text, t)!,
      muted: Color.lerp(muted, other.muted, t)!,
      running: Color.lerp(running, other.running, t)!,
      deploying: Color.lerp(deploying, other.deploying, t)!,
      warning: Color.lerp(warning, other.warning, t)!,
      failed: Color.lerp(failed, other.failed, t)!,
      stopped: Color.lerp(stopped, other.stopped, t)!,
    );
  }
}
