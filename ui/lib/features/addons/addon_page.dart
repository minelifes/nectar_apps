import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../api/client.dart';
import '../../api/models.dart';
import '../../api/session.dart';
import '../../core/format.dart';
import '../../data/providers.dart';
import '../../generated/l10n.dart';
import '../../theme/theme.dart';
import '../../theme/tokens.dart';
import '../../widgets/common.dart';
import '../../widgets/copy.dart';
import '../../widgets/gauge.dart';
import '../../widgets/status.dart';
import '../apps/terminal_tab.dart';
import '../observe/app_metrics.dart';
import '../observe/event_list.dart';
import '../observe/log_viewer.dart';

/// One database add-on, managed like an app: overview (connection, apps, resources), metrics, logs,
/// terminal and events; stop / start / restart / delete.
class AddonPage extends ConsumerWidget {
  const AddonPage({super.key, required this.id});

  final int id;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = S.of(context);
    final addon = ref.watch(addonProvider(id));
    Future<void> refresh() async => ref.invalidate(addonProvider(id));
    return addon.when(
      skipLoadingOnRefresh: true,
      loading: () => const LoadingBlock(height: 320),
      error: (e, _) => PageScaffold(
        title: s.addonDatabase,
        children: [ErrorBlock(error: e, onRetry: refresh)],
      ),
      data: (a) => _Body(addon: a, onRefresh: refresh),
    );
  }
}

class _Body extends ConsumerStatefulWidget {
  const _Body({required this.addon, required this.onRefresh});

  final Addon addon;
  final Future<void> Function() onRefresh;

  @override
  ConsumerState<_Body> createState() => _BodyState();
}

class _BodyState extends ConsumerState<_Body> {
  int _tab = 0;

  Addon get addon => widget.addon;

  Future<void> _op(BuildContext context, WidgetRef ref, String op) async {
    try {
      await ref.read(apiProvider).post('/addons/${addon.id}/$op');
      ref.invalidate(addonProvider(addon.id));
      ref.invalidate(projectAddonsProvider(addon.projectId));
    } on Object catch (e) {
      if (context.mounted) showSnack(context, describeError(e), error: true);
    }
  }

  Future<void> _delete(BuildContext context, WidgetRef ref) async {
    final s = S.of(context);
    final ok = await confirmAction(
      context,
      s.addonDelete(addon.name),
      s.addonTheDatabaseAndAllIts,
      s.addonDeleteDatabase,
      danger: true,
    );
    if (!ok || !context.mounted) return;
    try {
      await ref.read(apiProvider).delete('/addons/${addon.id}');
      ref.invalidate(projectAddonsProvider(addon.projectId));
      if (context.mounted) context.go('/projects/${addon.projectId}');
    } on Object catch (e) {
      if (context.mounted) showSnack(context, describeError(e), error: true);
    }
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final session = ref.watch(sessionProvider);
    bool can(String p) => session.can(p, projectId: addon.projectId);
    final c = NectarColors.of(context);
    final path = '/addons/${addon.id}';
    final tabs = <(String, Widget Function())>[
      (
        s.addonTabOverview,
        () => Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            _ConnectionCard(addon: addon, canReveal: can('env.secrets.view')),
            const SizedBox(height: NectarTokens.gap),
            _LinksCard(addon: addon, canEdit: can('env.edit')),
            const SizedBox(height: NectarTokens.gap),
            _ResourcesCard(
              key: ValueKey(
                '${addon.sizeMb}-${addon.memoryMb}-${addon.cpuMillicores}',
              ),
              addon: addon,
              canEdit: can('app.edit'),
            ),
          ],
        ),
      ),
      if (can('app.metrics')) (s.addonTabMetrics, () => UsageCard(path: path)),
      if (can('app.logs'))
        (
          s.addonTabLogs,
          () => NCard(
            child: LogViewer(
              key: ValueKey('addon-logs-${addon.id}'),
              path: path,
            ),
          ),
        ),
      if (can('app.exec') && !addon.stopped)
        (
          s.addonTabTerminal,
          () => NCard(
            child: TerminalTab(
              key: ValueKey('addon-term-${addon.id}'),
              path: path,
            ),
          ),
        ),
      (s.addonTabEvents, () => EventsCard(path: path)),
    ];
    final tab = _tab.clamp(0, tabs.length - 1);
    return PageScaffold(
      title: addon.name,
      subtitle: '${addon.kindName} ${addon.version} · ${addon.slug}',
      leading: IconButton(
        tooltip: s.projectProject,
        icon: const Icon(Icons.arrow_back_rounded),
        onPressed: () => context.go('/projects/${addon.projectId}'),
      ),
      onRefresh: widget.onRefresh,
      actions: [
        if (can('app.restart') && !addon.stopped)
          OutlinedButton(
            onPressed: () => _op(context, ref, 'restart'),
            child: Text(s.appRestart),
          ),
        if (can('app.restart'))
          OutlinedButton(
            onPressed: () =>
                _op(context, ref, addon.stopped ? 'start' : 'stop'),
            child: Text(addon.stopped ? 'Start' : 'Stop'),
          ),
        if (can('app.delete'))
          IconButton(
            tooltip: s.actionDelete,
            icon: const Icon(Icons.delete_outline_rounded),
            onPressed: () => _delete(context, ref),
          ),
      ],
      children: [
        Row(
          children: [
            StatusPill(status: addon.liveStatus),
            if (addon.problem.isNotEmpty || addon.statusMessage.isNotEmpty) ...[
              const SizedBox(width: 10),
              Expanded(
                child: Text(
                  addon.problem.isNotEmpty
                      ? addon.problem
                      : addon.statusMessage,
                  style: TextStyle(color: c.muted, fontSize: 13),
                ),
              ),
            ],
          ],
        ),
        const SizedBox(height: NectarTokens.gap),
        NTabs(
          index: tab,
          labels: [for (final t in tabs) t.$1],
          onChanged: (i) => setState(() => _tab = i),
        ),
        const SizedBox(height: NectarTokens.gap),
        tabs[tab].$2(),
      ],
    );
  }
}

class _ConnectionCard extends ConsumerStatefulWidget {
  const _ConnectionCard({required this.addon, required this.canReveal});

  final Addon addon;
  final bool canReveal;

  @override
  ConsumerState<_ConnectionCard> createState() => _ConnectionCardState();
}

class _ConnectionCardState extends ConsumerState<_ConnectionCard> {
  Map<String, dynamic>? _creds;

  Future<void> _reveal() async {
    try {
      final res = await ref
          .read(apiProvider)
          .get('/addons/${widget.addon.id}/credentials');
      if (mounted) {
        setState(() => _creds = Map<String, dynamic>.from(res as Map));
      }
    } on Object catch (e) {
      if (mounted) showSnack(context, describeError(e), error: true);
    }
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final a = widget.addon;
    final c = NectarColors.of(context);
    final creds = _creds;
    final env = creds == null
        ? null
        : Map<String, dynamic>.from(creds['env'] as Map? ?? const {});
    final rows = <(String, String)>[
      ('Host', '${a.host}:${a.port}'),
      if (creds != null && '${creds['user']}'.isNotEmpty)
        ('User', '${creds['user']}'),
      if (creds != null && '${creds['database']}'.isNotEmpty)
        ('Database', '${creds['database']}'),
      if (creds != null) ('Password', '${creds['password']}'),
      if (env != null)
        for (final k in env.keys.where((k) => k.endsWith('_URL')))
          (k, '${env[k]}'),
    ];
    return NCard(
      title: s.addonConnection,
      trailing: creds == null && widget.canReveal
          ? TextButton.icon(
              onPressed: _reveal,
              icon: const Icon(Icons.visibility_outlined, size: 18),
              label: Text(s.addonShowCredentials),
            )
          : null,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          for (final (k, v) in rows)
            Padding(
              padding: const EdgeInsets.symmetric(vertical: 4),
              child: Row(
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
                      style: monoStyle(context, size: 13),
                    ),
                  ),
                  IconButton(
                    tooltip: s.backupCopy,
                    icon: const Icon(Icons.copy_rounded, size: 16),
                    onPressed: () {
                      copyText(context, v, done: s.backupCopied);
                    },
                  ),
                ],
              ),
            ),
          const SizedBox(height: 6),
          Text(
            s.addonReachableOnlyFromAppsIn(a.host, a.port),
            style: TextStyle(color: c.muted, fontSize: 12.5),
          ),
        ],
      ),
    );
  }
}

class _LinksCard extends ConsumerWidget {
  const _LinksCard({required this.addon, required this.canEdit});

  final Addon addon;
  final bool canEdit;

  Future<void> _link(
    BuildContext context,
    WidgetRef ref,
    List<App> apps,
  ) async {
    final s = S.of(context);
    final linked = {for (final l in addon.links) l.$1};
    final candidates = apps.where((a) => !linked.contains(a.id)).toList();
    if (candidates.isEmpty) {
      showSnack(context, s.addonEveryAppOfTheProject);
      return;
    }
    App? app = candidates.first;
    final prefix = TextEditingController();
    final ok = await showDialog<bool>(
      context: context,
      builder: (ctx) => StatefulBuilder(
        builder: (ctx, set) => AlertDialog(
          title: Text(s.addonLinkToAnApp(addon.name)),
          content: SizedBox(
            width: 420,
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                DropdownButtonFormField<App>(
                  initialValue: app,
                  decoration: InputDecoration(labelText: s.addonApp),
                  items: [
                    for (final a in candidates)
                      DropdownMenuItem(value: a, child: Text(a.name)),
                  ],
                  onChanged: (v) => set(() => app = v),
                ),
                const SizedBox(height: 12),
                TextField(
                  controller: prefix,
                  textCapitalization: TextCapitalization.characters,
                  decoration: InputDecoration(
                    labelText: s.addonVariablePrefixOptional,
                    hintText: s.addonCache,
                    helperText: s.addonForASecondDatabaseCache,
                  ),
                ),
              ],
            ),
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.pop(ctx, false),
              child: Text(s.actionCancel),
            ),
            FilledButton(
              onPressed: () => Navigator.pop(ctx, true),
              child: Text(s.addonLink),
            ),
          ],
        ),
      ),
    );
    final p = prefix.text.trim();
    prefix.dispose();
    if (ok != true || app == null || !context.mounted) return;
    try {
      await ref
          .read(apiProvider)
          .post(
            '/addons/${addon.id}/links',
            data: {
              'app_id': app!.id,
              'prefix': p.isEmpty || p.endsWith('_') ? p : '${p}_',
            },
          );
      ref.invalidate(addonProvider(addon.id));
      ref.invalidate(appAddonsProvider(app!.id));
      if (context.mounted) {
        showSnack(context, s.addonLinkedRedeployToApply(app!.name));
      }
    } on Object catch (e) {
      if (context.mounted) showSnack(context, describeError(e), error: true);
    }
  }

  /// Shares the database with every app of the project (asks for the variable prefix), or stops sharing.
  Future<void> _share(BuildContext context, WidgetRef ref, bool on) async {
    final s = S.of(context);
    var prefix = addon.sharedPrefix;
    if (on) {
      final ctl = TextEditingController(text: addon.sharedPrefix);
      final ok = await showDialog<bool>(
        context: context,
        builder: (ctx) => AlertDialog(
          title: Text(s.addonShareWithProject),
          content: SizedBox(
            width: 420,
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                Text(
                  s.addonShareHelp,
                  style: TextStyle(
                    color: NectarColors.of(ctx).muted,
                    fontSize: 13,
                  ),
                ),
                const SizedBox(height: 12),
                TextField(
                  controller: ctl,
                  autofocus: true,
                  textCapitalization: TextCapitalization.characters,
                  decoration: InputDecoration(
                    labelText: s.addonSharePrefix,
                    hintText: s.addonCache,
                    helperText: s.addonForASecondDatabaseCache,
                  ),
                ),
              ],
            ),
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.pop(ctx, false),
              child: Text(s.actionCancel),
            ),
            FilledButton(
              onPressed: () => Navigator.pop(ctx, true),
              child: Text(s.actionSave),
            ),
          ],
        ),
      );
      final p = ctl.text.trim().toUpperCase();
      ctl.dispose();
      if (ok != true || !context.mounted) return;
      prefix = p.isEmpty || p.endsWith('_') ? p : '${p}_';
    }
    try {
      await ref
          .read(apiProvider)
          .put(
            '/addons/${addon.id}/shared',
            data: {'shared': on, 'prefix': prefix},
          );
      ref.invalidate(addonProvider(addon.id));
      ref.invalidate(projectAddonsProvider(addon.projectId));
      if (context.mounted) showSnack(context, s.addonSharedSaved);
    } on Object catch (e) {
      if (context.mounted) showSnack(context, describeError(e), error: true);
    }
  }

  Future<void> _unlink(BuildContext context, WidgetRef ref, int appId) async {
    try {
      await ref.read(apiProvider).delete('/addons/${addon.id}/links/$appId');
      ref.invalidate(addonProvider(addon.id));
      ref.invalidate(appAddonsProvider(appId));
    } on Object catch (e) {
      if (context.mounted) showSnack(context, describeError(e), error: true);
    }
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = S.of(context);
    final apps = ref.watch(projectAppsProvider(addon.projectId));
    final byId = apps.hasValue
        ? {for (final a in apps.value!) a.id: a}
        : const <int, App>{};
    final c = NectarColors.of(context);
    return NCard(
      title: s.addonLinkedApps,
      trailing: canEdit && apps.hasValue
          ? TextButton.icon(
              onPressed: () => _link(context, ref, apps.value!),
              icon: const Icon(Icons.link_rounded, size: 18),
              label: Text(s.addonLinkAnApp),
            )
          : null,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          SwitchListTile(
            contentPadding: EdgeInsets.zero,
            title: Text(s.addonShareWithProject),
            subtitle: Text(
              addon.shared
                  ? s.addonSharedAs(
                      addon.envNames
                          .map((n) => '${addon.sharedPrefix}$n')
                          .join(', '),
                    )
                  : s.addonShareHelp,
              style: addon.shared
                  ? monoStyle(context, size: 11.5, color: c.muted)
                  : TextStyle(color: c.muted, fontSize: 12.5),
            ),
            secondary: addon.shared && canEdit
                ? IconButton(
                    tooltip: s.addonSharePrefix,
                    icon: const Icon(Icons.edit_outlined, size: 18),
                    onPressed: () => _share(context, ref, true),
                  )
                : null,
            value: addon.shared,
            onChanged: canEdit ? (v) => _share(context, ref, v) : null,
          ),
          if (addon.shared && addon.links.isNotEmpty)
            Padding(
              padding: const EdgeInsets.only(bottom: 4),
              child: Text(
                s.addonLinkOverridesShared,
                style: TextStyle(color: c.muted, fontSize: 12),
              ),
            ),
          if (addon.links.isEmpty && !addon.shared)
            Text(
              s.addonNoAppUsesThisDatabase,
              style: TextStyle(color: c.muted, fontSize: 13),
            ),
          for (final (appId, prefix) in addon.links)
            ListTile(
              contentPadding: EdgeInsets.zero,
              leading: const Icon(Icons.apps_rounded),
              title: Text(byId[appId]?.name ?? 'App #$appId'),
              subtitle: Text(
                addon.envNames.map((n) => '$prefix$n').join(', '),
                style: monoStyle(context, size: 11.5, color: c.muted),
              ),
              onTap: () => context.go('/apps/$appId'),
              trailing: canEdit
                  ? IconButton(
                      tooltip: s.addonUnlink,
                      icon: const Icon(Icons.link_off_rounded, size: 18),
                      onPressed: () => _unlink(context, ref, appId),
                    )
                  : null,
            ),
        ],
      ),
    );
  }
}

class _ResourcesCard extends ConsumerStatefulWidget {
  const _ResourcesCard({super.key, required this.addon, required this.canEdit});

  final Addon addon;
  final bool canEdit;

  @override
  ConsumerState<_ResourcesCard> createState() => _ResourcesCardState();
}

class _ResourcesCardState extends ConsumerState<_ResourcesCard> {
  late final _size = TextEditingController(text: '${widget.addon.sizeMb}');
  late final _mem = TextEditingController(text: '${widget.addon.memoryMb}');
  late final _cpu = TextEditingController(
    text: '${widget.addon.cpuMillicores}',
  );
  bool _saving = false;

  @override
  void dispose() {
    _size.dispose();
    _mem.dispose();
    _cpu.dispose();
    super.dispose();
  }

  Future<void> _save() async {
    final s = S.of(context);
    setState(() => _saving = true);
    try {
      await ref
          .read(apiProvider)
          .patch(
            '/addons/${widget.addon.id}',
            data: {
              'size_mb': int.tryParse(_size.text.trim()) ?? 0,
              'memory_mb': int.tryParse(_mem.text.trim()) ?? 0,
              'cpu_millicores': int.tryParse(_cpu.text.trim()) ?? 0,
            },
          );
      ref.invalidate(addonProvider(widget.addon.id));
      if (mounted) showSnack(context, s.addonSavedTheDatabaseRestartsWith);
    } on Object catch (e) {
      if (mounted) showSnack(context, describeError(e), error: true);
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final s = S.of(context);
    final a = widget.addon;
    Widget num(TextEditingController ctl, String label, String help) =>
        SizedBox(
          width: 180,
          child: TextField(
            controller: ctl,
            enabled: widget.canEdit,
            keyboardType: TextInputType.number,
            inputFormatters: [FilteringTextInputFormatter.digitsOnly],
            decoration: InputDecoration(labelText: label, helperText: help),
          ),
        );
    return NCard(
      title: s.projectResources,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Wrap(
            spacing: 16,
            runSpacing: 12,
            children: [
              num(
                _size,
                'Storage (MiB)',
                'Can only grow; now ${formatMb(a.sizeMb)}',
              ),
              num(_mem, 'Memory (MiB)', 'Limit'),
              num(_cpu, 'CPU (millicores)', '1000 = one core'),
            ],
          ),
          if (a.usedBytes >= 0 && a.sizeMb > 0)
            Padding(
              padding: const EdgeInsets.only(top: 14),
              child: UsageBar(
                label: s.appUsed,
                value: a.usedBytes / (a.sizeMb * 1024 * 1024),
                trailing:
                    '${formatBytes(a.usedBytes)} of ${formatMb(a.sizeMb)}',
              ),
            ),
          if (widget.canEdit)
            Align(
              alignment: Alignment.centerRight,
              child: Padding(
                padding: const EdgeInsets.only(top: 12),
                child: FilledButton(
                  onPressed: _saving ? null : _save,
                  child: Text(_saving ? 'Saving…' : 'Save'),
                ),
              ),
            ),
        ],
      ),
    );
  }
}
