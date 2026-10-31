import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../api/session.dart';
import '../data/providers.dart';
import '../generated/l10n.dart';
import '../theme/tokens.dart';
import '../widgets/hex.dart';
import 'version_banner.dart';

class _Dest {
  const _Dest(this.path, this.label, this.icon, this.selectedIcon);

  /// The label in the interface language.
  String text(BuildContext context) {
    final s = S.of(context);
    return switch (path) {
      '/' => s.navDashboard,
      '/projects' => s.navProjects,
      '/domains' => s.navDomains,
      '/server' => s.navServer,
      '/alerts' => s.navAlerts,
      '/backups' => s.navBackups,
      '/settings' => s.navSettings,
      _ => label,
    };
  }

  final String path;
  final String label;
  final IconData icon;
  final IconData selectedIcon;
}

const _all = [
  _Dest(
    '/',
    'Dashboard',
    Icons.space_dashboard_outlined,
    Icons.space_dashboard_rounded,
  ),
  _Dest('/projects', 'Projects', Icons.folder_outlined, Icons.folder_rounded),
  _Dest('/domains', 'Domains', Icons.language_outlined, Icons.language_rounded),
  _Dest('/server', 'Server', Icons.dns_outlined, Icons.dns_rounded),
  _Dest(
    '/alerts',
    'Alerts',
    Icons.notifications_outlined,
    Icons.notifications_rounded,
  ),
  _Dest('/backups', 'Backups', Icons.backup_outlined, Icons.backup_rounded),
  _Dest(
    '/settings',
    'Settings',
    Icons.settings_outlined,
    Icons.settings_rounded,
  ),
];

/// Destinations the user can open (Alerts needs server.view).
List<_Dest> _destinationsFor(SessionState s) => [
  for (final d in _all)
    if ((d.path != '/alerts' || s.can('server.view')) &&
        (d.path != '/backups' || s.can('backup.manage')))
      d,
];

int _selected(List<_Dest> dests, String location) {
  if (location == '/apps' ||
      location.startsWith('/apps/') ||
      location.startsWith('/addons/')) {
    return 1; // inside projects
  }
  for (final p in const ['/updates', '/users', '/roles', '/tokens']) {
    if (location.startsWith(p)) location = '/settings'; // reached from Settings
  }
  if (location.startsWith('/gateway')) location = '/server';
  for (var i = dests.length - 1; i > 0; i--) {
    if (location == dests[i].path || location.startsWith('${dests[i].path}/')) {
      return i;
    }
  }
  return 0;
}

/// Sidebar on wide screens (web/desktop/tablet landscape), bottom navigation on phones.
class AppShell extends ConsumerWidget {
  const AppShell({super.key, required this.location, required this.child});

  final String location;
  final Widget child;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final width = MediaQuery.sizeOf(context).width;
    final dests = _destinationsFor(ref.watch(sessionProvider));
    final firing = ref.watch(firingAlertsProvider);
    final alertCount =
        firing.value?.where((a) => !a.read).length ?? 0; // unread only
    final index = _selected(dests, location);
    if (width >= 900) {
      return Scaffold(
        body: Row(
          children: [
            _Sidebar(dests: dests, selected: index, alertCount: alertCount),
            Expanded(
              child: SafeArea(
                left: false,
                child: Column(
                  children: [
                    const VersionBanner(),
                    Expanded(child: child),
                  ],
                ),
              ),
            ),
          ],
        ),
      );
    }
    // phones: Backups is reached from Settings (six tabs do not fit)
    final mobile = dests.where((d) => d.path != '/backups').toList();
    final mIndex = _selected(
      mobile,
      location.startsWith('/backups') ? '/settings' : location,
    );
    return Scaffold(
      body: SafeArea(
        bottom: false,
        child: Column(
          children: [
            const VersionBanner(),
            Expanded(child: child),
          ],
        ),
      ),
      bottomNavigationBar: NavigationBar(
        selectedIndex: mIndex,
        onDestinationSelected: (i) => context.go(mobile[i].path),
        destinations: [
          for (final d in mobile)
            NavigationDestination(
              icon: _badged(d, alertCount, Icon(d.icon)),
              selectedIcon: _badged(d, alertCount, Icon(d.selectedIcon)),
              label: d.text(context),
            ),
        ],
      ),
    );
  }
}

Widget _badged(_Dest d, int count, Widget icon) =>
    d.path == '/alerts' && count > 0
    ? Badge(label: Text('$count'), child: icon)
    : icon;

class _Sidebar extends ConsumerWidget {
  const _Sidebar({
    required this.dests,
    required this.selected,
    required this.alertCount,
  });

  final List<_Dest> dests;
  final int selected;
  final int alertCount;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final c = NectarColors.of(context);
    final session = ref.watch(sessionProvider);
    final theme = Theme.of(context);
    return Container(
      width: 240,
      decoration: BoxDecoration(
        color: c.surface,
        border: Border(right: BorderSide(color: c.border)),
      ),
      child: SafeArea(
        right: false,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 22, 20, 26),
              child: Row(
                children: [
                  const NectarMark(size: 30),
                  const SizedBox(width: 10),
                  Text(
                    'Nectar Apps',
                    style: theme.textTheme.titleLarge?.copyWith(
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                ],
              ),
            ),
            for (var i = 0; i < dests.length; i++)
              _NavItem(
                dest: dests[i],
                selected: i == selected,
                badge: dests[i].path == '/alerts' ? alertCount : 0,
              ),
            const Spacer(),
            if (session.user != null) _UserChip(session: session),
          ],
        ),
      ),
    );
  }
}

class _NavItem extends StatefulWidget {
  const _NavItem({required this.dest, required this.selected, this.badge = 0});

  final _Dest dest;
  final bool selected;
  final int badge;

  @override
  State<_NavItem> createState() => _NavItemState();
}

class _NavItemState extends State<_NavItem> {
  bool _hover = false;

  @override
  Widget build(BuildContext context) {
    final c = NectarColors.of(context);
    final sel = widget.selected;
    final color = sel ? NectarTokens.amber : (_hover ? c.text : c.muted);
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 2),
      child: MouseRegion(
        onEnter: (_) => setState(() => _hover = true),
        onExit: (_) => setState(() => _hover = false),
        child: Material(
          color: sel
              ? NectarTokens.amber.withValues(alpha: 0.12)
              : (_hover ? c.elevated : Colors.transparent),
          borderRadius: BorderRadius.circular(NectarTokens.radiusSmall),
          child: InkWell(
            borderRadius: BorderRadius.circular(NectarTokens.radiusSmall),
            onTap: () => context.go(widget.dest.path),
            child: Padding(
              padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
              child: Row(
                children: [
                  Icon(
                    sel ? widget.dest.selectedIcon : widget.dest.icon,
                    size: 20,
                    color: color,
                  ),
                  const SizedBox(width: 12),
                  Text(
                    widget.dest.text(context),
                    style: TextStyle(
                      fontSize: 14,
                      fontWeight: sel ? FontWeight.w600 : FontWeight.w500,
                      color: sel ? c.text : color,
                    ),
                  ),
                  if (widget.badge > 0) ...[
                    const Spacer(),
                    Container(
                      padding: const EdgeInsets.symmetric(
                        horizontal: 7,
                        vertical: 1,
                      ),
                      decoration: BoxDecoration(
                        color: c.failed,
                        borderRadius: BorderRadius.circular(10),
                      ),
                      child: Text(
                        '${widget.badge}',
                        style: const TextStyle(
                          color: Colors.white,
                          fontSize: 11.5,
                          fontWeight: FontWeight.w700,
                        ),
                      ),
                    ),
                  ],
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}

class _UserChip extends ConsumerWidget {
  const _UserChip({required this.session});

  final SessionState session;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = S.of(context);
    final c = NectarColors.of(context);
    final user = session.user!;
    return Container(
      margin: const EdgeInsets.all(12),
      padding: const EdgeInsets.fromLTRB(10, 8, 4, 8),
      decoration: BoxDecoration(
        color: c.elevated,
        borderRadius: BorderRadius.circular(NectarTokens.radius),
        border: Border.all(color: c.border),
      ),
      child: Row(
        children: [
          HexBadge(
            size: 34,
            child: Text(
              user.initials,
              style: const TextStyle(
                fontSize: 12,
                fontWeight: FontWeight.w700,
                color: NectarTokens.onAmber,
              ),
            ),
          ),
          const SizedBox(width: 10),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  user.name.isEmpty ? user.email : user.name,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: const TextStyle(
                    fontSize: 13,
                    fontWeight: FontWeight.w600,
                  ),
                ),
                Text(
                  session.serverHost,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: TextStyle(fontSize: 11.5, color: c.muted),
                ),
              ],
            ),
          ),
          IconButton(
            tooltip: s.logout,
            icon: const Icon(Icons.logout_rounded, size: 18),
            onPressed: () => ref.read(sessionProvider.notifier).logout(),
          ),
        ],
      ),
    );
  }
}
