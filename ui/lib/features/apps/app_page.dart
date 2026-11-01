import 'dart:async';
import 'dart:math' as math;

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:nectar_apps_ui/features/observe/log_viewer.dart';
import 'package:url_launcher/url_launcher.dart';

import '../../api/models.dart';
import '../../api/session.dart';
import '../../core/format.dart';
import '../../data/providers.dart';
import '../../generated/l10n.dart';
import '../../theme/theme.dart';
import '../../theme/tokens.dart';
import '../../widgets/common.dart';
import '../../widgets/copy.dart';
import '../../widgets/hex.dart';
import '../../widgets/status.dart';
import '../alerts/alert_widgets.dart';
import '../observe/app_metrics.dart';
import '../observe/event_list.dart';
import '../observe/log_history.dart';
import '../projects/environments.dart';
import 'app_actions.dart';
import 'builds_tab.dart';
import 'env_tab.dart';
import 'ops_widgets.dart';
import 'settings_tab.dart';
import 'terminal_tab.dart';
import 'deploy_progress.dart';
import 'domain_dialog.dart';

/// App detail: Overview (live state, domains, configuration) and Deployments (history, rollback).
class AppPage extends ConsumerStatefulWidget {
  const AppPage({super.key, required this.id});

  final int id;

  @override
  ConsumerState<AppPage> createState() => _AppPageState();
}

class _AppPageState extends ConsumerState<AppPage> {
  String _tab = 'Overview';
  Timer? _timer;
  int? _watchRelease; // a deploy started from this page

  @override
  void initState() {
    super.initState();
    _timer = Timer.periodic(const Duration(seconds: 5), (_) => _tick());
  }

  @override
  void dispose() {
    _timer?.cancel();
    super.dispose();
  }

  void _tick() {
    // light polling: live state and releases; everything else on demand
    ref.invalidate(appLiveProvider(widget.id));
    ref.invalidate(releasesProvider(widget.id));
    ref.invalidate(appProvider(widget.id));
  }

  Future<void> _refresh() async => refreshApp(ref, widget.id);

  void _started(Release? r) {
    if (r == null) return;
    setState(() => _watchRelease = r.id);
    refreshApp(ref, widget.id);
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final app = ref.watch(appProvider(widget.id));
    final back = IconButton(
      tooltip: s.projectProject,
      icon: const Icon(Icons.arrow_back_rounded),
      onPressed: () => app.hasValue
          ? context.go('/projects/${app.value!.projectId}')
          : context.go('/projects'),
    );
    return app.when(
      skipLoadingOnRefresh: true,
      skipLoadingOnReload: true,
      loading: () => const LoadingBlock(height: 320),
      error: (e, _) => PageScaffold(
        title: s.addonApp,
        leading: back,
        children: [ErrorBlock(error: e, onRetry: _refresh)],
      ),
      data: (a) => _body(a, back),
    );
  }

  Widget _body(App app, Widget back) {
    final s = S.of(context);
    final session = ref.watch(sessionProvider);
    bool can(String p) => session.can(p, projectId: app.projectId);
    final releases = ref.watch(releasesProvider(app.id));
    final rels = releases.hasValue ? releases.value! : const <Release>[];
    final running = rels.where((r) => r.inProgress).firstOrNull;
    final source = app.runtime == 'image'
        ? ref.watch(appSourceProvider(app.id)).value
        : null;
    final tabs = <(String, Widget Function())>[
      ('Overview', () => _Overview(app: app, onStarted: _started)),
      if (source != null) ('Builds', () => BuildsTab(app: app, source: source)),
      if (can('app.metrics'))
        ('Metrics', () => AppMetrics(app: app, canLogs: can('app.logs'))),
      if (can('app.logs'))
        (
          'Logs',
          () => NCard(
            child: LogsPanel(key: ValueKey('logs-${app.id}'), app: app),
          ),
        ),
      if (can('app.exec') && app.hasReplicas)
        (
          'Terminal',
          () => NCard(
            child: TerminalTab(
              key: ValueKey('term-${app.id}'),
              path: '/apps/${app.id}',
            ),
          ),
        ),
      ('Events', () => EventsCard(path: '/apps/${app.id}')),
      if (can('env.view'))
        ('Environment', () => EnvTab(key: ValueKey('env-${app.id}'), app: app)),
      (
        'Settings',
        () => SettingsTab(
          key: ValueKey('settings-${app.id}-${app.updatedAt}'),
          app: app,
          onDeployed: _started,
        ),
      ),
      (
        'Deployments',
        () => _Deployments(
          app: app,
          releases: releases,
          canRollback: can('app.rollback'),
          busy: running != null,
          onStarted: _started,
        ),
      ),
    ];
    final tab = math.max(0, tabs.indexWhere((t) => t.$1 == _tab));
    final watch = _watchRelease ?? running?.id;
    final rt = runtimeInfo(app.runtime);
    final neverDeployed = rels.isEmpty;

    final actions = <Widget>[
      if (can('app.deploy') && app.runtime != 'image')
        FilledButton.icon(
          onPressed: running != null
              ? null
              : () async => _started(await showUploadAndDeploy(context, app)),
          icon: const Icon(Icons.upload_rounded, size: 18),
          label: Text(neverDeployed ? 'Upload & deploy' : 'New version'),
        ),
      if (can('app.deploy') && app.runtime == 'image')
        FilledButton.icon(
          onPressed: running != null
              ? null
              : () async => _started(await showChangeImage(context, app)),
          icon: const Icon(Icons.rocket_launch_rounded, size: 18),
          label: Text(s.appDeployImage),
        ),
      _Menu(
        app: app,
        canDeploy: can('app.deploy'),
        canOperate: can('app.restart'),
        canDelete: can('app.delete'),
        busy: running != null,
        onStarted: _started,
      ),
    ];

    return PageScaffold(
      title: app.name,
      subtitle:
          '${rt.label}${app.runtimeVersion.isEmpty ? '' : ' ${app.runtimeVersion}'} · ${app.kind} · ${app.slug}',
      leading: back,
      onRefresh: _refresh,
      actions: actions,
      children: [
        Row(
          children: [
            StatusPill(status: app.status),
            if (app.statusMessage.isNotEmpty) ...[
              const SizedBox(width: 10),
              Expanded(
                child: Text(
                  app.statusMessage,
                  maxLines: 2,
                  overflow: TextOverflow.ellipsis,
                  style: TextStyle(
                    color: NectarColors.of(context).muted,
                    fontSize: 13,
                  ),
                ),
              ),
            ],
          ],
        ),
        const SizedBox(height: NectarTokens.gap),
        if (watch != null) ...[
          NCard(
            title: s.appDeployInProgress,
            trailing: _watchRelease != null && running == null
                ? IconButton(
                    tooltip: s.appHide,
                    icon: const Icon(Icons.close_rounded, size: 18),
                    onPressed: () => setState(() => _watchRelease = null),
                  )
                : null,
            child: DeployProgress(
              key: ValueKey(watch),
              app: app,
              releaseId: watch,
              onFinished: (_) => refreshApp(ref, app.id),
            ),
          ),
          const SizedBox(height: NectarTokens.gap),
        ],
        if (neverDeployed && watch == null)
          Padding(
            padding: const EdgeInsets.only(bottom: NectarTokens.gap),
            child: NCard(
              child: EmptyState(
                icon: Icons.rocket_launch_rounded,
                title: s.appNotDeployedYet,
                message: app.runtime == 'image'
                    ? 'Deploy the image to start the app.'
                    : 'Upload your file and deploy it to start the app.',
              ),
            ),
          ),
        NTabs(
          index: tab,
          labels: [for (final t in tabs) t.$1],
          onChanged: (i) => setState(() => _tab = tabs[i].$1),
        ),
        const SizedBox(height: NectarTokens.gap),
        tabs[tab].$2(),
      ],
    );
  }
}

class _Menu extends ConsumerWidget {
  const _Menu({
    required this.app,
    required this.canDeploy,
    required this.canOperate,
    required this.canDelete,
    required this.busy,
    required this.onStarted,
  });

  final App app;
  final bool canDeploy;
  final bool canOperate;
  final bool canDelete;
  final bool busy;
  final void Function(Release?) onStarted;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = S.of(context);
    final stopped = app.replicas == 0;
    final cron = app.kind == 'cron';
    final session = ref.watch(sessionProvider);
    final projects = ref.watch(projectsProvider).value ?? const <Project>[];
    final mine = projects.where((p) => p.id == app.projectId).firstOrNull;
    final targets = mine == null
        ? const <Project>[]
        : relatedProjects(
            mine,
            projects,
          ).where((p) => session.can('app.deploy', projectId: p.id)).toList();
    return PopupMenuButton<String>(
      tooltip: s.appMore,
      icon: const Icon(Icons.more_horiz_rounded),
      onSelected: (v) async {
        switch (v) {
          case 'redeploy':
            onStarted(await redeploy(context, ref, app));
          case 'restart':
            await appOperation(
              context,
              ref,
              app,
              'restart',
              done: 'Restarting ${app.name} pod by pod',
            );
          case 'stop':
            await appOperation(
              context,
              ref,
              app,
              'stop',
              done: cron ? 'Schedule paused' : '${app.name} stopped',
            );
          case 'start':
            await appOperation(
              context,
              ref,
              app,
              'start',
              done: cron ? 'Schedule resumed' : 'Starting ${app.name}',
            );
          case 'clone':
            await showCloneAppDialog(context, app);
          case 'delete':
            if (await deleteApp(context, ref, app) && context.mounted) {
              context.go('/projects/${app.projectId}');
            }
          default:
            if (v.startsWith('promote:')) {
              final t = targets
                  .where((p) => 'promote:${p.id}' == v)
                  .firstOrNull;
              if (t != null) await promoteApp(context, ref, app, t);
            }
        }
      },
      itemBuilder: (_) => [
        if (canDeploy)
          PopupMenuItem(
            value: 'redeploy',
            enabled: !busy,
            child: Text(s.appRedeployCurrentVersion),
          ),
        if (canOperate && app.hasReplicas && !stopped)
          PopupMenuItem(
            value: 'restart',
            enabled: !busy,
            child: Text(s.appRestart),
          ),
        if (canOperate && (app.hasReplicas || cron) && !stopped)
          PopupMenuItem(
            value: 'stop',
            enabled: !busy,
            child: Text(cron ? 'Pause schedule' : 'Stop'),
          ),
        if (canOperate && (app.hasReplicas || cron) && stopped)
          PopupMenuItem(
            value: 'start',
            enabled: !busy,
            child: Text(cron ? 'Resume schedule' : 'Start'),
          ),
        for (final t in targets)
          PopupMenuItem(
            value: 'promote:${t.id}',
            enabled: !busy,
            child: Text(s.appPromoteTo(envLabel(t))),
          ),
        PopupMenuItem(value: 'clone', child: Text(s.appCopyApp)),
        if (canDelete) const PopupMenuDivider(),
        if (canDelete)
          PopupMenuItem(value: 'delete', child: Text(s.appDeleteApp)),
      ],
    );
  }
}

// ---------------------------------------------------------------- overview --

class _Overview extends ConsumerWidget {
  const _Overview({required this.app, required this.onStarted});

  final App app;
  final void Function(Release?) onStarted;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final liveCard = _LiveCard(app: app);
    final domains = app.isWeb ? _DomainsCard(app: app) : null;
    final config = _ConfigCard(app: app);
    final alerts = ref.watch(appAlertsProvider(app.id));
    final layout = LayoutBuilder(
      builder: (context, box) {
        if (box.maxWidth < 980) {
          return Column(
            children: [
              if (domains != null)
                Padding(
                  padding: const EdgeInsets.only(bottom: NectarTokens.gap),
                  child: domains,
                ),
              liveCard,
              const SizedBox(height: NectarTokens.gap),
              config,
            ],
          );
        }
        return Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Expanded(
              flex: 3,
              child: Column(
                children: [
                  if (domains != null) ...[
                    domains,
                    const SizedBox(height: NectarTokens.gap),
                  ],
                  liveCard,
                ],
              ),
            ),
            const SizedBox(width: NectarTokens.gap),
            Expanded(flex: 2, child: config),
          ],
        );
      },
    );
    final live = ref.watch(appLiveProvider(app.id));
    final pending = live.hasValue ? live.value!.pending : const <String>[];
    final canDeploy = ref.watch(
      sessionProvider.select(
        (s) => s.can('app.deploy', projectId: app.projectId),
      ),
    );
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        AppAlertsBanner(value: alerts),
        if (pending.isNotEmpty)
          PendingBanner(
            app: app,
            pending: pending,
            canDeploy: canDeploy,
            onStarted: onStarted,
          ),
        layout,
      ],
    );
  }
}

class _LiveCard extends ConsumerWidget {
  const _LiveCard({required this.app});

  final App app;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l = S.of(context);
    final live = ref.watch(appLiveProvider(app.id));
    final canLogs = ref.watch(
      sessionProvider.select(
        (s) => s.can('app.logs', projectId: app.projectId),
      ),
    );
    final canScale = ref.watch(
      sessionProvider.select(
        (s) => s.can('app.restart', projectId: app.projectId),
      ),
    );
    final c = NectarColors.of(context);
    return NCard(
      title: l.statusRunning,
      trailing: live.hasValue && app.hasReplicas
          ? Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                Text(
                  l.appReady(live.value!.ready, live.value!.replicas),
                  style: TextStyle(color: c.muted, fontSize: 13),
                ),
                if (canScale) ...[
                  const SizedBox(width: 8),
                  ScaleControl(app: app),
                ],
              ],
            )
          : null,
      child: AsyncBlock(
        value: live,
        loadingHeight: 80,
        data: (s) {
          if (s.pods.isEmpty) {
            return Text(
              app.kind == 'cron'
                  ? 'Nothing running right now; the next run starts on schedule (${app.schedule}).'
                  : (app.replicas == 0 ? 'Stopped.' : 'No pods yet.'),
              style: TextStyle(color: c.muted, fontSize: 13.5),
            );
          }
          return Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              for (final p in s.pods)
                Padding(
                  padding: const EdgeInsets.symmetric(vertical: 6),
                  child: Row(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Padding(
                        padding: const EdgeInsets.only(top: 5),
                        child: StatusDot(
                          status: p.ready
                              ? 'running'
                              : (p.problem.isNotEmpty ? 'failed' : 'deploying'),
                        ),
                      ),
                      const SizedBox(width: 10),
                      Expanded(
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            Text(p.name, style: monoStyle(context, size: 12.5)),
                            Text(
                              [
                                p.phase,
                                if (p.release.isNotEmpty)
                                  'release #${p.release}',
                                if (p.restarts > 0) '${p.restarts} restarts',
                                if (p.startedAt != null)
                                  'started ${timeAgo(p.startedAt)}',
                              ].join(' · '),
                              style: TextStyle(color: c.muted, fontSize: 12),
                            ),
                            if (p.problem.isNotEmpty)
                              Text(
                                p.problem,
                                style: TextStyle(
                                  color: c.failed,
                                  fontSize: 12.5,
                                ),
                              ),
                          ],
                        ),
                      ),
                      if (p.restarts > 0 && canLogs)
                        TextButton.icon(
                          onPressed: () => showCrashLog(
                            context,
                            ref,
                            '/apps/${app.id}',
                            p.name,
                          ),
                          icon: const Icon(Icons.history_rounded, size: 16),
                          label: Text(l.appCrashLog),
                        ),
                    ],
                  ),
                ),
              for (final (port, proto, nodePort) in s.nodePorts)
                Padding(
                  padding: const EdgeInsets.only(top: 8),
                  child: Text(
                    l.appPortIsOpenOnThe(port, proto, nodePort),
                    style: const TextStyle(fontSize: 13),
                  ),
                ),
            ],
          );
        },
      ),
    );
  }
}

class _DomainsCard extends ConsumerWidget {
  const _DomainsCard({required this.app});

  final App app;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l = S.of(context);
    final domains = ref.watch(domainsProvider(app.id));
    final c = NectarColors.of(context);
    final canManage = ref.watch(
      sessionProvider.select(
        (s) => s.can('domain.manage', projectId: app.projectId),
      ),
    );
    return NCard(
      title: l.navDomains,
      trailing: canManage
          ? IconButton(
              tooltip: l.appAddDomain,
              icon: const Icon(Icons.add_link_rounded, size: 20),
              onPressed: () => showDomainDialog(context, appId: app.id),
            )
          : null,
      child: AsyncBlock(
        value: domains,
        loadingHeight: 60,
        data: (list) => list.isEmpty
            ? Text(
                l.appNoDomainSetABase,
                style: TextStyle(color: c.muted, fontSize: 13.5),
              )
            : Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  for (final d in list)
                    Padding(
                      padding: const EdgeInsets.symmetric(vertical: 6),
                      child: Row(
                        children: [
                          Icon(
                            d.tls ? Icons.lock_rounded : Icons.public_rounded,
                            size: 17,
                            color: d.status == 'active' ? c.running : c.muted,
                          ),
                          const SizedBox(width: 10),
                          Expanded(
                            child: Column(
                              crossAxisAlignment: CrossAxisAlignment.start,
                              children: [
                                InkWell(
                                  onTap: d.kind == 'app'
                                      ? () => launchUrl(Uri.parse(d.url))
                                      : null,
                                  child: Text(
                                    d.kind == 'redirect'
                                        ? '${d.host} → ${d.redirectTo}'
                                        : d.url,
                                    style: monoStyle(
                                      context,
                                      size: 13.5,
                                      color: d.kind == 'app'
                                          ? NectarTokens.amber
                                          : c.text,
                                    ),
                                  ),
                                ),
                                Text(
                                  [
                                    if (d.isDefault) 'default',
                                    if (d.statusMessage.isNotEmpty)
                                      d.statusMessage,
                                    if (d.certNotAfter != null)
                                      'certificate until ${d.certNotAfter!.toIso8601String().substring(0, 10)}',
                                    if (d.rateLimit > 0)
                                      '${d.rateLimit} req/s per IP',
                                  ].join(' · '),
                                  style: TextStyle(
                                    color: c.muted,
                                    fontSize: 12,
                                  ),
                                ),
                              ],
                            ),
                          ),
                          IconButton(
                            tooltip: l.backupCopy,
                            icon: const Icon(Icons.copy_rounded, size: 16),
                            onPressed: () {
                              copyText(
                                context,
                                d.url,
                                done: l.appCopied(d.host),
                              );
                            },
                          ),
                          if (canManage)
                            IconButton(
                              tooltip: l.backupEdit,
                              icon: const Icon(Icons.tune_rounded, size: 16),
                              onPressed: () => showDomainDialog(
                                context,
                                appId: app.id,
                                domain: d,
                              ),
                            ),
                          StatusPill(status: d.status),
                        ],
                      ),
                    ),
                ],
              ),
      ),
    );
  }
}

class _ConfigCard extends StatelessWidget {
  const _ConfigCard({required this.app});

  final App app;

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final c = NectarColors.of(context);
    final rows = <(String, String)>[
      (
        'Type',
        '${runtimeInfo(app.runtime).label}${app.runtimeVersion.isEmpty ? '' : ' ${app.runtimeVersion}'}',
      ),
      if (app.image.isNotEmpty) ('Image', app.image),
      ('Kind', app.kind),
      if (app.isWeb) ('Port', '${app.port}'),
      if (app.hasReplicas) ('Replicas', '${app.replicas}'),
      (
        'Resources',
        '${formatCpu(app.cpuMillicores)} · ${formatMb(app.memoryMb)} per replica',
      ),
      if (app.isWeb)
        (
          'Health check',
          app.healthType == 'http'
              ? 'HTTP ${app.healthPath}'
              : (app.healthType == 'tcp' ? 'port open' : 'none'),
        ),
      if (app.schedule.isNotEmpty) ('Schedule', app.schedule),
      ('Slug', app.slug),
      if (app.updatedAt != null) ('Changed', timeAgo(app.updatedAt)),
    ];
    return NCard(
      title: s.appConfiguration,
      child: Column(
        children: [
          for (final (k, v) in rows)
            Padding(
              padding: const EdgeInsets.symmetric(vertical: 5),
              child: Row(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  SizedBox(
                    width: 120,
                    child: Text(
                      k,
                      style: TextStyle(color: c.muted, fontSize: 13),
                    ),
                  ),
                  Expanded(
                    child: SelectableText(
                      v,
                      style: const TextStyle(fontSize: 13.5),
                    ),
                  ),
                ],
              ),
            ),
          const SizedBox(height: 6),
          Text(
            s.appChangeTheseOnTheSettings,
            style: TextStyle(color: c.muted, fontSize: 12),
          ),
        ],
      ),
    );
  }
}

// ------------------------------------------------------------- deployments --

class _Deployments extends ConsumerWidget {
  const _Deployments({
    required this.app,
    required this.releases,
    required this.canRollback,
    required this.busy,
    required this.onStarted,
  });

  final App app;
  final AsyncValue<List<Release>> releases;
  final bool canRollback;
  final bool busy;
  final void Function(Release?) onStarted;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = S.of(context);
    final artifacts = ref.watch(artifactsProvider(app.id));
    final byId = {
      for (final a
          in (artifacts.hasValue ? artifacts.value! : const <Artifact>[]))
        a.id: a,
    };
    final c = NectarColors.of(context);
    return NCard(
      padding: const EdgeInsets.symmetric(vertical: 6, horizontal: 18),
      child: AsyncBlock(
        value: releases,
        data: (list) {
          if (list.isEmpty) {
            return Padding(
              padding: const EdgeInsets.all(12),
              child: Text(
                s.appNoDeploymentsYet,
                style: TextStyle(color: c.muted),
              ),
            );
          }
          final liveNumber = list
              .where((r) => r.status == 'live')
              .firstOrNull
              ?.number;
          return Column(
            children: [
              for (final r in list)
                Container(
                  padding: const EdgeInsets.symmetric(vertical: 12),
                  decoration: BoxDecoration(
                    border: Border(
                      bottom: BorderSide(
                        color: c.border.withValues(alpha: 0.6),
                      ),
                    ),
                  ),
                  child: Row(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      HexBadge(
                        size: 34,
                        color: r.number == liveNumber
                            ? c.running.withValues(alpha: 0.18)
                            : c.elevated,
                        child: Text(
                          '${r.number}',
                          style: TextStyle(
                            fontSize: 12,
                            fontWeight: FontWeight.w700,
                            color: r.number == liveNumber ? c.running : c.muted,
                          ),
                        ),
                      ),
                      const SizedBox(width: 12),
                      Expanded(
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            Row(
                              children: [
                                StatusPill(
                                  status: r.status,
                                  label: r.number == liveNumber ? 'Live' : null,
                                ),
                                const SizedBox(width: 10),
                                Text(
                                  timeAgo(r.createdAt),
                                  style: TextStyle(
                                    color: c.muted,
                                    fontSize: 12.5,
                                  ),
                                ),
                              ],
                            ),
                            const SizedBox(height: 4),
                            Text(
                              r.artifactId != null && byId[r.artifactId] != null
                                  ? '${byId[r.artifactId]!.filename} · ${formatBytes(byId[r.artifactId]!.size)}'
                                  : (r.image.isNotEmpty ? r.image : ''),
                              style: monoStyle(
                                context,
                                size: 12.5,
                                color: c.muted,
                              ),
                            ),
                            if (r.message.isNotEmpty)
                              Padding(
                                padding: const EdgeInsets.only(top: 4),
                                child: Text(
                                  r.message,
                                  style: TextStyle(
                                    fontSize: 12.5,
                                    color: r.status == 'failed'
                                        ? c.failed
                                        : c.muted,
                                  ),
                                ),
                              ),
                          ],
                        ),
                      ),
                      if (canRollback &&
                          r.number != liveNumber &&
                          (r.status == 'live' || r.status == 'superseded'))
                        TextButton(
                          onPressed: busy
                              ? null
                              : () async => onStarted(
                                  await rollbackTo(context, ref, app, r),
                                ),
                          child: Text(s.updateRollBack2),
                        ),
                    ],
                  ),
                ),
            ],
          );
        },
      ),
    );
  }
}
