import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'api/session.dart';
import 'core/locale.dart';
import 'core/push.dart';
import 'core/theme_mode.dart';
import 'generated/l10n.dart';
import 'router.dart';
import 'theme/theme.dart';

class NectarApp extends ConsumerStatefulWidget {
  const NectarApp({super.key});

  @override
  ConsumerState<NectarApp> createState() => _NectarAppState();
}

class _NectarAppState extends ConsumerState<NectarApp> {
  StreamSubscription<String>? _taps;

  @override
  void initState() {
    super.initState();
    // a tapped push notification opens its page
    _taps = PushService.instance.taps.listen((path) => ref.read(routerProvider).go(path));
  }

  @override
  void dispose() {
    _taps?.cancel();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    // signed in → register this device for push notifications (when the panel has them on)
    ref.listen(sessionProvider.select((s) => s.status), (prev, next) {
      if (next == SessionStatus.signedIn && prev != next) {
        final api = ref.read(sessionProvider.notifier).api;
        if (api != null) unawaited(PushService.instance.register(api));
      }
    });
    return MaterialApp.router(
      title: 'Nectar Apps',
      debugShowCheckedModeBanner: false,
      theme: buildNectarTheme(Brightness.light),
      darkTheme: buildNectarTheme(Brightness.dark),
      themeMode: ref.watch(themeModeProvider),
      locale: ref.watch(localeProvider),
      supportedLocales: S.delegate.supportedLocales,
      localizationsDelegates: const [
        S.delegate,
        GlobalMaterialLocalizations.delegate,
        GlobalWidgetsLocalizations.delegate,
        GlobalCupertinoLocalizations.delegate,
      ],
      routerConfig: ref.watch(routerProvider),
    );
  }
}
