import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../api/client.dart';
import '../../api/models.dart';
import '../../api/session.dart';
import '../../core/format.dart';
import '../../data/providers.dart';
import '../../theme/theme.dart';
import '../../theme/tokens.dart';
import '../../widgets/common.dart';
import '../../widgets/status.dart';

/// One database add-on: connection, linked apps, resources, stop/start/delete.
class AddonPage extends ConsumerWidget {
  const AddonPage({super.key, required this.id});

  final int id;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final addon = ref.watch(addonProvider(id));
    Future<void> refresh() async => ref.invalidate(addonProvider(id));
    return addon.when(
      skipLoadingOnRefresh: true,
      loading: () => const LoadingBlock(height: 320),
      error: (e, _) => PageScaffold(title: 'Database', children: [ErrorBlock(error: e, onRetry: refresh)]),
      data: (a) => _Body(addon: a, onRefresh: refresh),
    );
  }
}

class _Body extends ConsumerWidget {
  const _Body({required this.addon, required this.onRefresh});

  final Addon addon;
  final Future<void> Function() onRefresh;

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
    final ok = await confirmAction(
      context,
      'Delete ${addon.name}?',
      'The database and all its data are deleted for good. Linked apps lose their connection settings with their next deploy.',
      'Delete database',
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
  Widget build(BuildContext context, WidgetRef ref) {
    final session = ref.watch(sessionProvider);
    bool can(String p) => session.can(p, projectId: addon.projectId);
    final c = NectarColors.of(context);
    return PageScaffold(
      title: addon.name,
      subtitle: '${addon.kindName} ${addon.version} · ${addon.slug}',
      leading: IconButton(
        tooltip: 'Project',
        icon: const Icon(Icons.arrow_back_rounded),
        onPressed: () => context.go('/projects/${addon.projectId}'),
      ),
      onRefresh: onRefresh,
      actions: [
        if (can('app.restart'))
          OutlinedButton(onPressed: () => _op(context, ref, addon.stopped ? 'start' : 'stop'), child: Text(addon.stopped ? 'Start' : 'Stop')),
        if (can('app.delete'))
          IconButton(tooltip: 'Delete', icon: const Icon(Icons.delete_outline_rounded), onPressed: () => _delete(context, ref)),
      ],
      children: [
        Row(
          children: [
            StatusPill(status: addon.liveStatus),
            if (addon.problem.isNotEmpty || addon.statusMessage.isNotEmpty) ...[
              const SizedBox(width: 10),
              Expanded(child: Text(addon.problem.isNotEmpty ? addon.problem : addon.statusMessage, style: TextStyle(color: c.muted, fontSize: 13))),
            ],
          ],
        ),
        const SizedBox(height: NectarTokens.gap),
        _ConnectionCard(addon: addon, canReveal: can('env.secrets.view')),
        const SizedBox(height: NectarTokens.gap),
        _LinksCard(addon: addon, canEdit: can('env.edit')),
        const SizedBox(height: NectarTokens.gap),
        _ResourcesCard(key: ValueKey('${addon.sizeMb}-${addon.memoryMb}-${addon.cpuMillicores}'), addon: addon, canEdit: can('app.edit')),
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
      final res = await ref.read(apiProvider).get('/addons/${widget.addon.id}/credentials');
      if (mounted) setState(() => _creds = Map<String, dynamic>.from(res as Map));
    } on Object catch (e) {
      if (mounted) showSnack(context, describeError(e), error: true);
    }
  }

  @override
  Widget build(BuildContext context) {
    final a = widget.addon;
    final c = NectarColors.of(context);
    final creds = _creds;
    final env = creds == null ? null : Map<String, dynamic>.from(creds['env'] as Map? ?? const {});
    final rows = <(String, String)>[
      ('Host', '${a.host}:${a.port}'),
      if (creds != null && '${creds['user']}'.isNotEmpty) ('User', '${creds['user']}'),
      if (creds != null && '${creds['database']}'.isNotEmpty) ('Database', '${creds['database']}'),
      if (creds != null) ('Password', '${creds['password']}'),
      if (env != null)
        for (final k in env.keys.where((k) => k.endsWith('_URL'))) (k, '${env[k]}'),
    ];
    return NCard(
      title: 'Connection',
      trailing: creds == null && widget.canReveal
          ? TextButton.icon(onPressed: _reveal, icon: const Icon(Icons.visibility_outlined, size: 18), label: const Text('Show credentials'))
          : null,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          for (final (k, v) in rows)
            Padding(
              padding: const EdgeInsets.symmetric(vertical: 4),
              child: Row(
                children: [
                  SizedBox(width: 120, child: Text(k, style: TextStyle(color: c.muted, fontSize: 13))),
                  Expanded(child: SelectableText(v, style: monoStyle(context, size: 13))),
                  IconButton(
                    tooltip: 'Copy',
                    icon: const Icon(Icons.copy_rounded, size: 16),
                    onPressed: () {
                      Clipboard.setData(ClipboardData(text: v));
                      showSnack(context, 'Copied');
                    },
                  ),
                ],
              ),
            ),
          const SizedBox(height: 6),
          Text(
            'Reachable only from apps in the same project, at ${a.host}:${a.port}. Link an app below to give it these settings as variables.',
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

  Future<void> _link(BuildContext context, WidgetRef ref, List<App> apps) async {
    final linked = {for (final l in addon.links) l.$1};
    final candidates = apps.where((a) => !linked.contains(a.id)).toList();
    if (candidates.isEmpty) {
      showSnack(context, 'Every app of the project is linked already');
      return;
    }
    App? app = candidates.first;
    final prefix = TextEditingController();
    final ok = await showDialog<bool>(
      context: context,
      builder: (ctx) => StatefulBuilder(
        builder: (ctx, set) => AlertDialog(
          title: Text('Link ${addon.name} to an app'),
          content: SizedBox(
            width: 420,
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                DropdownButtonFormField<App>(
                  initialValue: app,
                  decoration: const InputDecoration(labelText: 'App'),
                  items: [for (final a in candidates) DropdownMenuItem(value: a, child: Text(a.name))],
                  onChanged: (v) => set(() => app = v),
                ),
                const SizedBox(height: 12),
                TextField(
                  controller: prefix,
                  textCapitalization: TextCapitalization.characters,
                  decoration: const InputDecoration(
                    labelText: 'Variable prefix (optional)',
                    hintText: 'CACHE_',
                    helperText: 'For a second database: CACHE_ gives CACHE_DATABASE_URL',
                  ),
                ),
              ],
            ),
          ),
          actions: [
            TextButton(onPressed: () => Navigator.pop(ctx, false), child: const Text('Cancel')),
            FilledButton(onPressed: () => Navigator.pop(ctx, true), child: const Text('Link')),
          ],
        ),
      ),
    );
    final p = prefix.text.trim();
    prefix.dispose();
    if (ok != true || app == null || !context.mounted) return;
    try {
      await ref.read(apiProvider).post('/addons/${addon.id}/links', data: {'app_id': app!.id, 'prefix': p.isEmpty || p.endsWith('_') ? p : '${p}_'});
      ref.invalidate(addonProvider(addon.id));
      ref.invalidate(appAddonsProvider(app!.id));
      if (context.mounted) showSnack(context, 'Linked. Redeploy ${app!.name} to apply.');
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
    final apps = ref.watch(projectAppsProvider(addon.projectId));
    final byId = apps.hasValue ? {for (final a in apps.value!) a.id: a} : const <int, App>{};
    final c = NectarColors.of(context);
    return NCard(
      title: 'Linked apps',
      trailing: canEdit && apps.hasValue
          ? TextButton.icon(onPressed: () => _link(context, ref, apps.value!), icon: const Icon(Icons.link_rounded, size: 18), label: const Text('Link an app'))
          : null,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          if (addon.links.isEmpty) Text('No app uses this database yet.', style: TextStyle(color: c.muted, fontSize: 13)),
          for (final (appId, prefix) in addon.links)
            ListTile(
              contentPadding: EdgeInsets.zero,
              leading: const Icon(Icons.apps_rounded),
              title: Text(byId[appId]?.name ?? 'App #$appId'),
              subtitle: Text(addon.envNames.map((n) => '$prefix$n').join(', '), style: monoStyle(context, size: 11.5, color: c.muted)),
              onTap: () => context.go('/apps/$appId'),
              trailing: canEdit
                  ? IconButton(tooltip: 'Unlink', icon: const Icon(Icons.link_off_rounded, size: 18), onPressed: () => _unlink(context, ref, appId))
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
  late final _cpu = TextEditingController(text: '${widget.addon.cpuMillicores}');
  bool _saving = false;

  @override
  void dispose() {
    _size.dispose();
    _mem.dispose();
    _cpu.dispose();
    super.dispose();
  }

  Future<void> _save() async {
    setState(() => _saving = true);
    try {
      await ref.read(apiProvider).patch('/addons/${widget.addon.id}', data: {
        'size_mb': int.tryParse(_size.text.trim()) ?? 0,
        'memory_mb': int.tryParse(_mem.text.trim()) ?? 0,
        'cpu_millicores': int.tryParse(_cpu.text.trim()) ?? 0,
      });
      ref.invalidate(addonProvider(widget.addon.id));
      if (mounted) showSnack(context, 'Saved; the database restarts with the new resources');
    } on Object catch (e) {
      if (mounted) showSnack(context, describeError(e), error: true);
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final a = widget.addon;
    Widget num(TextEditingController ctl, String label, String help) => SizedBox(
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
      title: 'Resources',
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Wrap(
            spacing: 16,
            runSpacing: 12,
            children: [
              num(_size, 'Storage (MiB)', 'Can only grow; now ${formatMb(a.sizeMb)}'),
              num(_mem, 'Memory (MiB)', 'Limit'),
              num(_cpu, 'CPU (millicores)', '1000 = one core'),
            ],
          ),
          if (widget.canEdit)
            Align(
              alignment: Alignment.centerRight,
              child: Padding(
                padding: const EdgeInsets.only(top: 12),
                child: FilledButton(onPressed: _saving ? null : _save, child: Text(_saving ? 'Saving…' : 'Save')),
              ),
            ),
        ],
      ),
    );
  }
}
