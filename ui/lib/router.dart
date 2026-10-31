import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import 'api/session.dart';
import 'features/addons/addon_page.dart';
import 'features/alerts/alerts_page.dart';
import 'features/apps/app_page.dart';
import 'features/apps/new_app_page.dart';
import 'features/auth/connect_page.dart';
import 'features/backups/backups_page.dart';
import 'features/auth/login_page.dart';
import 'features/auth/reset_page.dart';
import 'features/auth/setup_page.dart';
import 'features/auth/splash_page.dart';
import 'features/dashboard/dashboard_page.dart';
import 'features/domains/domains_page.dart';
import 'features/onboarding/onboarding_page.dart';
import 'features/projects/project_page.dart';
import 'features/projects/projects_page.dart';
import 'features/server/server_page.dart';
import 'features/access/roles_page.dart';
import 'features/access/tokens_page.dart';
import 'features/access/users_page.dart';
import 'features/gateway/gateway_page.dart';
import 'features/settings/settings_page.dart';
import 'features/updates/updates_page.dart';
import 'generated/l10n.dart';
import 'shell/app_shell.dart';
import 'widgets/common.dart';

const publicPaths = {'/splash', '/connect', '/login', '/setup'};

final routerProvider = Provider<GoRouter>((ref) {
  final session = ValueNotifier<SessionState>(ref.read(sessionProvider));
  ref.listen(sessionProvider, (_, next) => session.value = next);

  Page<void> page(Widget child) => NoTransitionPage<void>(child: child);

  final router = GoRouter(
    initialLocation: '/',
    refreshListenable: session,
    redirect: (context, state) =>
        sessionRedirect(session.value, state.matchedLocation, state.uri),
    routes: [
      GoRoute(path: '/splash', builder: (context, state) => const SplashPage()),
      GoRoute(
        path: '/connect',
        builder: (context, state) => const ConnectPage(),
      ),
      GoRoute(path: '/login', builder: (context, state) => const LoginPage()),
      GoRoute(
        path: '/reset',
        builder: (context, state) =>
            ResetPage(token: state.uri.queryParameters['token'] ?? ''),
      ),
      GoRoute(
        path: '/setup',
        builder: (context, state) =>
            SetupPage(token: setupTokenFrom(state.uri)),
      ),
      GoRoute(
        path: '/welcome',
        builder: (context, state) => const OnboardingPage(),
      ),
      ShellRoute(
        builder: (context, state, child) =>
            AppShell(location: state.matchedLocation, child: child),
        routes: [
          GoRoute(
            path: '/',
            pageBuilder: (context, state) => page(const DashboardPage()),
          ),
          GoRoute(
            path: '/projects',
            pageBuilder: (context, state) => page(const ProjectsPage()),
            routes: [
              GoRoute(
                path: ':pid',
                pageBuilder: (context, state) => page(
                  ProjectPage(
                    id: int.tryParse(state.pathParameters['pid'] ?? '') ?? 0,
                  ),
                ),
              ),
            ],
          ),
          GoRoute(
            path: '/apps/new',
            pageBuilder: (context, state) => page(
              NewAppPage(
                projectId: int.tryParse(
                  state.uri.queryParameters['project'] ?? '',
                ),
              ),
            ),
          ),
          GoRoute(
            path: '/apps/:id',
            pageBuilder: (context, state) => page(
              AppPage(id: int.tryParse(state.pathParameters['id'] ?? '') ?? 0),
            ),
          ),
          GoRoute(
            path: '/server',
            pageBuilder: (context, state) => page(const ServerPage()),
          ),
          GoRoute(
            path: '/domains',
            pageBuilder: (context, state) => page(const DomainsPage()),
          ),
          GoRoute(
            path: '/addons/:id',
            pageBuilder: (context, state) => page(
              AddonPage(
                id: int.tryParse(state.pathParameters['id'] ?? '') ?? 0,
              ),
            ),
          ),
          GoRoute(
            path: '/backups',
            pageBuilder: (context, state) => page(const BackupsPage()),
          ),
          GoRoute(
            path: '/alerts',
            pageBuilder: (context, state) => page(const AlertsPage()),
          ),
          GoRoute(
            path: '/settings',
            pageBuilder: (context, state) => page(const SettingsPage()),
          ),
          GoRoute(
            path: '/updates',
            pageBuilder: (context, state) => page(const UpdatesPage()),
          ),
          GoRoute(
            path: '/users',
            pageBuilder: (context, state) => page(const UsersPage()),
          ),
          GoRoute(
            path: '/roles',
            pageBuilder: (context, state) => page(const RolesPage()),
          ),
          GoRoute(
            path: '/tokens',
            pageBuilder: (context, state) => page(const TokensPage()),
          ),
          GoRoute(
            path: '/gateway',
            pageBuilder: (context, state) => page(const GatewayPage()),
          ),
        ],
      ),
    ],
    errorBuilder: (context, state) => Scaffold(
      body: Center(
        child: EmptyState(
          icon: Icons.explore_off_rounded,
          title: S.of(context).pageNotFound,
          message: state.uri.path,
          action: FilledButton(
            onPressed: () => context.go('/'),
            child: Text(S.of(context).goToDashboard),
          ),
        ),
      ),
    ),
  );
  ref.onDispose(() {
    router.dispose();
    session.dispose();
  });
  return router;
});

/// Sends the user to the page their session state needs, remembering where they wanted to go
/// (`?from=`), and back there once they are signed in.
String? sessionRedirect(SessionState s, String location, Uri uri) {
  if (location == '/reset') {
    return null; // a reset or invitation link works signed in or out
  }
  final isPublic = publicPaths.contains(location);
  final wanted = isPublic
      ? _safe(
          uri.queryParameters['from'] ??
              (location == '/setup' ? uri.toString() : '/'),
        )
      : uri.toString();

  String? go(String page) {
    if (location == page) return null;
    return wanted == '/' ? page : '$page?from=${Uri.encodeComponent(wanted)}';
  }

  return switch (s.status) {
    SessionStatus.loading => go('/splash'),
    SessionStatus.needsServer => go('/connect'),
    SessionStatus.needsSetup => go('/setup'),
    SessionStatus.signedOut || SessionStatus.unreachable => go('/login'),
    SessionStatus.signedIn => isPublic ? _afterSignIn(wanted) : null,
  };
}

/// A destination after sign-in must not be one of the sign-in pages (that would loop).
String _afterSignIn(String wanted) {
  final path = Uri.tryParse(wanted)?.path ?? '/';
  return publicPaths.contains(path) ? '/' : wanted;
}

String _safe(String from) => from.startsWith('/') ? from : '/';

/// The setup token from `/setup?token=…` (the link install.sh prints), also when it was carried in `from`.
String? setupTokenFrom(Uri uri) {
  final direct = uri.queryParameters['token'];
  if (direct != null && direct.isNotEmpty) return direct;
  final from = uri.queryParameters['from'];
  if (from == null) return null;
  return Uri.tryParse(from)?.queryParameters['token'];
}
