import 'package:flutter/material.dart';

import 'tokens.dart';

/// Builds the Material 3 theme for one brightness from the brand tokens.
ThemeData buildNectarTheme(Brightness brightness) {
  final dark = brightness == Brightness.dark;
  final c = dark ? NectarColors.dark : NectarColors.light;

  final scheme = ColorScheme(
    brightness: brightness,
    primary: NectarTokens.amber,
    onPrimary: NectarTokens.onAmber,
    primaryContainer: dark ? const Color(0xFF3A2A0C) : const Color(0xFFFFE8BF),
    onPrimaryContainer: dark ? NectarTokens.pollen : const Color(0xFF4A3000),
    secondary: NectarTokens.pollen,
    onSecondary: NectarTokens.onAmber,
    error: c.failed,
    onError: Colors.white,
    surface: c.surface,
    onSurface: c.text,
    onSurfaceVariant: c.muted,
    surfaceContainerLowest: c.background,
    surfaceContainerLow: c.surface,
    surfaceContainer: c.surface,
    surfaceContainerHigh: c.elevated,
    surfaceContainerHighest: c.elevated,
    outline: c.border,
    outlineVariant: c.border,
  );

  final radius = BorderRadius.circular(NectarTokens.radius);
  final base = ThemeData(useMaterial3: true, colorScheme: scheme, brightness: brightness, fontFamily: NectarTokens.fontUi);

  TextStyle display(double size, FontWeight w, {double spacing = -0.4}) =>
      TextStyle(fontFamily: NectarTokens.fontDisplay, fontSize: size, fontWeight: w, letterSpacing: spacing, color: c.text);

  final text = base.textTheme
      .apply(bodyColor: c.text, displayColor: c.text, fontFamily: NectarTokens.fontUi)
      .copyWith(
        displaySmall: display(34, FontWeight.w700),
        headlineMedium: display(26, FontWeight.w700),
        headlineSmall: display(22, FontWeight.w700, spacing: -0.2),
        titleLarge: display(19, FontWeight.w500, spacing: -0.1),
        titleMedium: TextStyle(fontSize: 15, fontWeight: FontWeight.w600, color: c.text),
        titleSmall: TextStyle(fontSize: 13, fontWeight: FontWeight.w600, color: c.text),
        bodyLarge: TextStyle(fontSize: 15, height: 1.5, color: c.text),
        bodyMedium: TextStyle(fontSize: 14, height: 1.45, color: c.text),
        bodySmall: TextStyle(fontSize: 12.5, height: 1.4, color: c.muted),
        labelLarge: const TextStyle(fontSize: 14, fontWeight: FontWeight.w600),
        labelMedium: TextStyle(fontSize: 12, fontWeight: FontWeight.w600, color: c.muted, letterSpacing: 0.2),
        labelSmall: TextStyle(fontSize: 11, fontWeight: FontWeight.w600, color: c.muted, letterSpacing: 0.6),
      );

  final inputBorder = OutlineInputBorder(borderRadius: radius, borderSide: BorderSide(color: c.border));

  return base.copyWith(
    scaffoldBackgroundColor: c.background,
    canvasColor: c.background,
    textTheme: text,
    extensions: [c],
    dividerTheme: DividerThemeData(color: c.border, thickness: 1, space: 1),
    cardTheme: CardThemeData(
      color: c.surface,
      elevation: 0,
      margin: EdgeInsets.zero,
      shape: RoundedRectangleBorder(borderRadius: radius, side: BorderSide(color: c.border)),
    ),
    appBarTheme: AppBarTheme(
      backgroundColor: c.background,
      foregroundColor: c.text,
      elevation: 0,
      scrolledUnderElevation: 0,
      centerTitle: false,
      titleTextStyle: text.titleLarge,
    ),
    inputDecorationTheme: InputDecorationTheme(
      filled: true,
      fillColor: dark ? c.background : c.elevated,
      contentPadding: const EdgeInsets.symmetric(horizontal: 14, vertical: 14),
      border: inputBorder,
      enabledBorder: inputBorder,
      focusedBorder: inputBorder.copyWith(borderSide: const BorderSide(color: NectarTokens.amber, width: 1.5)),
      errorBorder: inputBorder.copyWith(borderSide: BorderSide(color: c.failed)),
      focusedErrorBorder: inputBorder.copyWith(borderSide: BorderSide(color: c.failed, width: 1.5)),
      labelStyle: TextStyle(color: c.muted),
      hintStyle: TextStyle(color: c.muted),
    ),
    filledButtonTheme: FilledButtonThemeData(
      style: FilledButton.styleFrom(
        backgroundColor: NectarTokens.amber,
        foregroundColor: NectarTokens.onAmber,
        disabledBackgroundColor: c.border,
        minimumSize: const Size(0, 44),
        padding: const EdgeInsets.symmetric(horizontal: 20),
        shape: RoundedRectangleBorder(borderRadius: radius),
        textStyle: const TextStyle(fontSize: 14, fontWeight: FontWeight.w600),
      ).copyWith(
        overlayColor: WidgetStateProperty.resolveWith(
          (s) => s.contains(WidgetState.hovered) ? NectarTokens.amberHover.withValues(alpha: 0.35) : null,
        ),
      ),
    ),
    outlinedButtonTheme: OutlinedButtonThemeData(
      style: OutlinedButton.styleFrom(
        foregroundColor: c.text,
        side: BorderSide(color: c.border),
        minimumSize: const Size(0, 44),
        padding: const EdgeInsets.symmetric(horizontal: 18),
        shape: RoundedRectangleBorder(borderRadius: radius),
        textStyle: const TextStyle(fontSize: 14, fontWeight: FontWeight.w600),
      ),
    ),
    textButtonTheme: TextButtonThemeData(
      style: TextButton.styleFrom(
        foregroundColor: NectarTokens.amber,
        shape: RoundedRectangleBorder(borderRadius: radius),
        textStyle: const TextStyle(fontSize: 14, fontWeight: FontWeight.w600),
      ),
    ),
    iconButtonTheme: IconButtonThemeData(style: IconButton.styleFrom(foregroundColor: c.muted)),
    chipTheme: ChipThemeData(
      backgroundColor: c.elevated,
      side: BorderSide(color: c.border),
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(NectarTokens.radiusSmall)),
      labelStyle: TextStyle(fontSize: 12.5, color: c.text),
    ),
    dialogTheme: DialogThemeData(
      backgroundColor: c.surface,
      shape: RoundedRectangleBorder(borderRadius: radius, side: BorderSide(color: c.border)),
      titleTextStyle: text.titleLarge,
    ),
    snackBarTheme: SnackBarThemeData(
      behavior: SnackBarBehavior.floating,
      backgroundColor: c.elevated,
      contentTextStyle: TextStyle(color: c.text),
      shape: RoundedRectangleBorder(borderRadius: radius, side: BorderSide(color: c.border)),
    ),
    tooltipTheme: TooltipThemeData(
      decoration: BoxDecoration(color: c.elevated, borderRadius: BorderRadius.circular(6), border: Border.all(color: c.border)),
      textStyle: TextStyle(color: c.text, fontSize: 12),
    ),
    navigationRailTheme: NavigationRailThemeData(
      backgroundColor: c.surface,
      indicatorColor: NectarTokens.amber.withValues(alpha: 0.16),
      selectedIconTheme: const IconThemeData(color: NectarTokens.amber),
      unselectedIconTheme: IconThemeData(color: c.muted),
    ),
    navigationBarTheme: NavigationBarThemeData(
      backgroundColor: c.surface,
      indicatorColor: NectarTokens.amber.withValues(alpha: 0.16),
      height: 64,
      iconTheme: WidgetStateProperty.resolveWith(
        (s) => IconThemeData(color: s.contains(WidgetState.selected) ? NectarTokens.amber : c.muted),
      ),
      labelTextStyle: WidgetStateProperty.resolveWith(
        (s) => TextStyle(
          fontSize: 11.5,
          fontWeight: FontWeight.w600,
          color: s.contains(WidgetState.selected) ? c.text : c.muted,
        ),
      ),
    ),
    progressIndicatorTheme: ProgressIndicatorThemeData(color: NectarTokens.amber, linearTrackColor: c.border),
  );
}

/// Monospace style for logs, env values and identifiers.
TextStyle monoStyle(BuildContext context, {double size = 13, Color? color}) => TextStyle(
      fontFamily: NectarTokens.fontMono,
      fontSize: size,
      height: 1.45,
      color: color ?? NectarColors.of(context).text,
    );
