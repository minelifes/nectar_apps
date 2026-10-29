import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'core/theme_mode.dart';
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
      routerConfig: ref.watch(routerProvider),
    );
  }
}
