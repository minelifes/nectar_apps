import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../api/models.dart';
import '../../api/session.dart';
import '../../data/providers.dart';
import '../../generated/l10n.dart';
import '../../theme/tokens.dart';
import '../../widgets/app_tile.dart';
import '../../widgets/activity.dart';
import '../../widgets/common.dart';
import '../../widgets/status.dart';
import '../alerts/alert_widgets.dart';
import '../projects/new_project_dialog.dart';
import '../server/node_card.dart';

class DashboardPage extends ConsumerStatefulWidget {
  const DashboardPage({super.key});

  @override
  ConsumerState<DashboardPage> createState() => _DashboardPageState();
}

/// The onboarding wizard opens by itself once per app start (not again after "skip" in the same session).
bool _onboardingShown = false;

class _DashboardPageState extends ConsumerState<DashboardPage> {
  Timer? _timer;

  @override
  void initState() {
    super.initState();
    // live-ish: refresh node usage, app status and activity every 15 seconds
    _timer = Timer.periodic(const Duration(seconds: 15), (_) => _refresh());
  }

  @override
  void dispose() {
    _timer?.cancel();
    super.dispose();
  }

  Future<void> _refresh() async {
    ref.invalidate(nodesProvider);
    ref.invalidate(projectsProvider);
    ref.invalidate(projectAppsProvider);
    ref.invalidate(recentActivityProvider);
    try {
      await ref.read(projectsWithAppsProvider.future);
    } on Object {
      // shown by the page
    }
  }

  String _greeting(String name) {
    final h = DateTime.now().hour;
    final part = h < 5 ? 'evening' : (h < 12 ? 'morning' : (h < 18 ? 'afternoon' : 'evening'));
    final first = name.trim().split(' ').first;
    return first.isEmpty ? 'Good $part' : 'Good $part, $first';
  }

  @override
  Widget build(BuildContext context) {
    final l = S.of(context);
    final session = ref.watch(sessionProvider);
    final canServer = session.can('server.view');
    final projects = ref.watch(projectsWithAppsProvider);
    final activity = ref.watch(recentActivityProvider);
    final wide = MediaQuery.sizeOf(context).width >= 1180;
    final onboarding = ref.watch(onboardingProvider);
    final needsSetup = onboarding.hasValue && onboarding.value != null && onboarding.value!['done'] != true;
    if (needsSetup && !_onboardingShown) {
      _onboardingShown = true;
      WidgetsBinding.instance.addPostFrameCallback((_) {
        if (mounted) context.go('/welcome');
      });
    }

    final firing = ref.watch(firingAlertsProvider);
    final server = canServer ? _ServerSection(nodes: ref.watch(nodesProvider)) : null;
    final entries = activity.hasValue ? activity.value : null;
    final feed = entries == null ? null : ActivityCard(entries: entries);

    return PageScaffold(
      title: _greeting(session.user?.name ?? ''),
      subtitle: [
        session.serverHost,
        if (session.version != null) 'Nectar Apps ${session.version!.version}',
      ].where((s) => s.isNotEmpty).join(' · '),
      onRefresh: _refresh,
      actions: [
        if (session.permissions.canAnywhere('app.create'))
          FilledButton.icon(
            onPressed: () => context.go('/apps/new'),
            icon: const Icon(Icons.add_rounded, size: 18),
            label: Text(l.newApp),
          ),
      ],
      children: [
        if (needsSetup)
          Padding(
            padding: const EdgeInsets.only(bottom: NectarTokens.gap),
            child: NCard(
              child: Row(
                children: [
                  const Icon(Icons.flag_rounded, color: NectarTokens.amber),
                  const SizedBox(width: 12),
                  Expanded(child: Text(l.dashboardFinishSettingUpDomainHttps)),
                  FilledButton(onPressed: () => context.go('/welcome'), child: Text(l.dashboardContinueSetup)),
                ],
              ),
            ),
          ),
        if (firing.hasValue) FiringAlertsCard(alerts: firing.value!),
        projects.maybeWhen(data: (list) => _Stats(items: list), orElse: () => const SizedBox.shrink()),
        if (wide && (server != null || feed != null))
          Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              if (server != null) Expanded(flex: 3, child: server),
              if (server != null && feed != null) const SizedBox(width: NectarTokens.gap),
              if (feed != null) Expanded(flex: 2, child: feed),
            ],
          )
        else
          ?server,
        const SizedBox(height: NectarTokens.gapLarge),
        SectionLabel(
          'Projects',
          trailing: TextButton(onPressed: () => context.go('/projects'), child: Text(l.projectAllProjects)),
        ),
        AsyncBlock(
          value: projects,
          onRetry: _refresh,
          data: (list) => list.isEmpty
              ? _NoProjects(canCreate: session.can('project.create'))
              : Column(children: [for (final (p, apps) in list) _ProjectApps(project: p, apps: apps)]),
        ),
        if (!wide && feed != null) ...[const SizedBox(height: NectarTokens.gapLarge), feed],
      ],
    );
  }
}

class _ServerSection extends StatelessWidget {
  const _ServerSection({required this.nodes});

  final AsyncValue<List<NodeInfo>> nodes;

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    return nodes.when(
        skipLoadingOnRefresh: true,
        data: (list) => Column(
          children: [
            for (final n in list) Padding(padding: const EdgeInsets.only(bottom: NectarTokens.gap), child: NodeCard(node: n)),
          ],
        ),
        loading: () => NCard(title: s.navServer, child: LoadingBlock(height: 140)),
        error: (e, _) => NCard(
          title: s.navServer,
          child: ErrorBlock(error: e, compact: true),
        ),
      );
  }
}

class _Stats extends StatelessWidget {
  const _Stats({required this.items});

  final List<(Project, List<App>)> items;

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final apps = [for (final (_, a) in items) ...a];
    final running = apps.where((a) => toneFor(a.status) == StatusTone.running).length;
    final failing = apps.where((a) => toneFor(a.status) == StatusTone.failed || toneFor(a.status) == StatusTone.warning).length;
    final deploying = apps.where((a) => a.status == 'deploying').length;
    return Padding(
      padding: const EdgeInsets.only(bottom: NectarTokens.gap),
      child: Wrap(
        spacing: NectarTokens.gap,
        runSpacing: NectarTokens.gap,
        children: [
          _StatTile(label: s.navProjects, value: '${items.length}', icon: Icons.folder_rounded),
          _StatTile(label: s.projectApps, value: '${apps.length}', icon: Icons.apps_rounded),
          _StatTile(label: s.statusRunning, value: '$running', icon: Icons.check_circle_rounded, tone: StatusTone.running),
          if (deploying > 0) _StatTile(label: s.statusDeploying, value: '$deploying', icon: Icons.sync_rounded, tone: StatusTone.deploying),
          _StatTile(
            label: s.dashboardNeedAttention,
            value: '$failing',
            icon: Icons.warning_amber_rounded,
            tone: failing > 0 ? StatusTone.failed : StatusTone.stopped,
          ),
        ],
      ),
    );
  }
}

class _StatTile extends StatelessWidget {
  const _StatTile({required this.label, required this.value, required this.icon, this.tone});

  final String label;
  final String value;
  final IconData icon;
  final StatusTone? tone;

  @override
  Widget build(BuildContext context) {
    final c = NectarColors.of(context);
    final color = tone == null ? NectarTokens.amber : toneColor(context, tone!);
    final theme = Theme.of(context);
    return Container(
      width: 170,
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: c.surface,
        borderRadius: BorderRadius.circular(NectarTokens.radius),
        border: Border.all(color: c.border),
      ),
      child: Row(
        children: [
          Icon(icon, color: color, size: 22),
          const SizedBox(width: 12),
          Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(value, style: theme.textTheme.headlineSmall?.copyWith(height: 1.1)),
              Text(label, style: theme.textTheme.bodySmall),
            ],
          ),
        ],
      ),
    );
  }
}

class _ProjectApps extends StatelessWidget {
  const _ProjectApps({required this.project, required this.apps});

  final Project project;
  final List<App> apps;

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final c = NectarColors.of(context);
    final theme = Theme.of(context);
    return Padding(
      padding: const EdgeInsets.only(bottom: 22),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          InkWell(
            borderRadius: BorderRadius.circular(NectarTokens.radiusSmall),
            onTap: () => context.go('/projects/${project.id}'),
            child: Padding(
              padding: const EdgeInsets.symmetric(vertical: 6, horizontal: 2),
              child: Row(
                mainAxisSize: MainAxisSize.min,
                children: [
                  Text(project.name, style: theme.textTheme.titleMedium),
                  const SizedBox(width: 8),
                  Text(project.namespace, style: TextStyle(color: c.muted, fontSize: 12.5)),
                  Icon(Icons.chevron_right_rounded, size: 18, color: c.muted),
                ],
              ),
            ),
          ),
          const SizedBox(height: 8),
          if (apps.isEmpty)
            Text(s.dashboardNoAppsYet, style: TextStyle(color: c.muted, fontSize: 13.5))
          else
            Wrap(
              spacing: 12,
              runSpacing: 12,
              children: [for (final a in apps) AppTile(app: a, onTap: () => context.go('/apps/${a.id}'))],
            ),
        ],
      ),
    );
  }
}

class _NoProjects extends ConsumerWidget {
  const _NoProjects({required this.canCreate});

  final bool canCreate;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = S.of(context);
    return NCard(
        child: EmptyState(
          icon: Icons.rocket_launch_rounded,
          title: s.dashboardNothingDeployedYet,
          message: s.dashboardAProjectIsASpace,
          action: canCreate
              ? FilledButton.icon(
                  onPressed: () => showNewProjectDialog(context, ref),
                  icon: const Icon(Icons.add_rounded, size: 18),
                  label: Text(s.projectNewProject),
                )
              : null,
        ),
      );
  }
}
