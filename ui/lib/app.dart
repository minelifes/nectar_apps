import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'core/locale.dart';
import 'core/theme_mode.dart';
import 'l10n/app_localizations.dart';
import 'router.dart';
import 'theme/theme.dart';

class NectarApp extends ConsumerWidget {
  const NectarApp({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return MaterialApp.router(
      title: 'Nectar Apps',
      debugShowCheckedModeBanner: false,
      theme: buildNectarTheme(Brightness.light),
      darkTheme: buildNectarTheme(Brightness.dark),
      themeMode: ref.watch(themeModeProvider),
      locale: ref.watch(localeProvider),
      supportedLocales: AppLocalizations.supportedLocales,
      localizationsDelegates: const [
        AppLocalizations.delegate,
        GlobalMaterialLocalizations.delegate,
        GlobalWidgetsLocalizations.delegate,
        GlobalCupertinoLocalizations.delegate,
      ],
      routerConfig: ref.watch(routerProvider),
    );
  }
}
